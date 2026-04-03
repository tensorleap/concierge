package validate

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/tensorleap/concierge/internal/core"
)

const (
	integrationTestASTEvidenceSummary = "guide.integration_test_ast.json"
	integrationTestASTEvidenceCommand = "guide.integration_test_ast.command"
	integrationTestASTEvidenceStdout  = "guide.integration_test_ast.stdout"
	integrationTestASTEvidenceStderr  = "guide.integration_test_ast.stderr"
)

const integrationTestASTScript = `
import ast
import json
import sys


MANUAL_BATCH_NAMES = {
    "expand_dims",
    "unsqueeze",
}
TRANSFORM_NAMES = {
    "argmax",
    "clip",
    "decode",
    "format",
    "reshape",
    "sigmoid",
    "softmax",
    "squeeze",
    "stack",
    "threshold",
    "transpose",
    "concatenate",
    "concat",
    "vstack",
    "hstack",
}
LIBRARY_ROOTS = {
    "jax",
    "np",
    "numpy",
    "pd",
    "pandas",
    "tf",
    "tensorflow",
    "torch",
}


def _decorator_name(node):
    if isinstance(node, ast.Call):
        return _decorator_name(node.func)
    if isinstance(node, ast.Name):
        return node.id
    if isinstance(node, ast.Attribute):
        return node.attr
    return ""


def _leaf_name(node):
    if isinstance(node, ast.Name):
        return node.id
    if isinstance(node, ast.Attribute):
        return node.attr
    return ""


def _root_name(node):
    current = node
    while isinstance(current, ast.Attribute):
        current = current.value
    if isinstance(current, ast.Name):
        return current.id
    return ""


def _call_name(node):
    if isinstance(node, ast.Name):
        return node.id
    if isinstance(node, ast.Attribute):
        return node.attr
    if isinstance(node, ast.Call):
        return _call_name(node.func)
    return ""


def _annotation_name(node):
    if isinstance(node, ast.Constant) and isinstance(node.value, str):
        return node.value
    if isinstance(node, ast.Name):
        return node.id
    if isinstance(node, ast.Attribute):
        return node.attr
    if isinstance(node, ast.Subscript):
        return _annotation_name(node.value)
    return ""


def _bind_names(target):
    if isinstance(target, ast.Name):
        return [target.id]
    if isinstance(target, (ast.Tuple, ast.List)):
        names = []
        for item in target.elts:
            names.extend(_bind_names(item))
        return names
    return []


def _load_model_runtime(function_node):
    annotation = _annotation_name(getattr(function_node, "returns", None)).strip().lower()
    if annotation == "inferencesession":
        return "onnx_session"

    session_values = set()
    for node in ast.walk(function_node):
        if isinstance(node, ast.Assign) and isinstance(node.value, ast.Call) and _call_name(node.value.func).lower() == "inferencesession":
            for target in node.targets:
                session_values.update(_bind_names(target))
        if isinstance(node, ast.AnnAssign) and isinstance(node.value, ast.Call) and _call_name(node.value.func).lower() == "inferencesession":
            session_values.update(_bind_names(node.target))

    for node in ast.walk(function_node):
        if not isinstance(node, ast.Return) or node.value is None:
            continue
        if isinstance(node.value, ast.Call) and _call_name(node.value.func).lower() == "inferencesession":
            return "onnx_session"
        if isinstance(node.value, ast.Name) and node.value.id in session_values:
            return "onnx_session"

    return ""


def _infer_symbol(function_name):
    value = (function_name or "").strip().lower()
    for prefix in ("encode_", "input_", "gt_", "label_", "target_", "metadata_"):
        if value.startswith(prefix):
            return value[len(prefix):]
    return value


def _extract_symbol(decorator, function_name):
    if not isinstance(decorator, ast.Call):
        return _infer_symbol(function_name)

    for keyword in decorator.keywords:
        if keyword.arg and keyword.arg.lower() in {"input", "feature", "target", "name"}:
            if isinstance(keyword.value, ast.Constant) and isinstance(keyword.value.value, str):
                return keyword.value.value.strip().lower()

    if decorator.args:
        first = decorator.args[0]
        if isinstance(first, ast.Constant) and isinstance(first.value, str):
            return first.value.strip().lower()

    return _infer_symbol(function_name)


def _function_kind(name):
    normalized = (name or "").strip().lower()
    if normalized == "tensorleap_input_encoder":
        return "input_encoder"
    if normalized == "tensorleap_gt_encoder":
        return "gt_encoder"
    if normalized == "tensorleap_load_model":
        return "load_model"
    if normalized == "tensorleap_integration_test":
        return "integration_test"
    if normalized.startswith("tensorleap_"):
        return normalized[len("tensorleap_"):]
    return ""


def _decorated_functions(tree):
    functions = []
    integration_tests = []
    kind_by_function = {}

    for node in tree.body:
        if not isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)):
            continue

        for decorator in node.decorator_list:
            name = _decorator_name(decorator)
            kind = _function_kind(name)
            if not kind:
                continue

            entry = {
                "function": node.name,
                "kind": kind,
                "line": getattr(node, "lineno", 0) or 0,
            }
            if kind == "load_model":
                runtime = _load_model_runtime(node)
                if runtime:
                    entry["runtime"] = runtime
            if kind in {"input_encoder", "gt_encoder", "metadata", "custom_metric", "custom_loss", "custom_visualizer"}:
                entry["symbol"] = _extract_symbol(decorator, node.name)
            functions.append(entry)
            kind_by_function[node.name] = kind
            if kind == "integration_test":
                integration_tests.append(node)

    return functions, integration_tests, kind_by_function


class _IntegrationFunctionAnalyzer:
    def __init__(self, node, decorated_functions, kind_by_function):
        self.node = node
        self.decorated_by_function = {entry["function"]: entry for entry in decorated_functions}
        self.kind_by_function = kind_by_function
        self.allowed_decorated_functions = set(self.decorated_by_function.keys())
        self.load_model_functions = {
            entry["function"]
            for entry in decorated_functions
            if entry["kind"] == "load_model"
        }
        self.load_model_runtime = {
            entry["function"]: entry.get("runtime", "")
            for entry in decorated_functions
            if entry["kind"] == "load_model"
        }
        self.sample_arg = ""
        self.preprocess_arg = ""
        positional_args = list(getattr(node.args, "args", []) or [])
        if len(positional_args) > 0:
            self.sample_arg = positional_args[0].arg
        if len(positional_args) > 1:
            self.preprocess_arg = positional_args[1].arg

        self.allowed_values = set()
        self.model_values = set()
        self.model_runtime_by_value = {}
        self.model_metadata_values = set()
        self.prediction_values = set()
        self.calls = []
        self.prediction_uses = []
        self.unknown_calls = []
        self.direct_dataset_access = []
        self.manual_batch = []
        self.illegal_body_logic = []

    def result(self):
        return {
            "function": self.node.name,
            "line": getattr(self.node, "lineno", 0) or 0,
            "arguments": [arg.arg for arg in getattr(self.node.args, "args", []) or []],
            "calls": self.calls,
            "predictionUses": self.prediction_uses,
            "unknownCalls": self.unknown_calls,
            "directDatasetAccess": self.direct_dataset_access,
            "manualBatchManipulations": self.manual_batch,
            "illegalBodyLogic": self.illegal_body_logic,
        }

    def record(self, bucket, name, line, kind, detail):
        bucket.append(
            {
                "name": (name or "").strip(),
                "line": int(line or 0),
                "kind": (kind or "").strip(),
                "detail": (detail or "").strip(),
            }
        )

    def record_prediction_use(self, expr, kind, detail):
        name = ""
        if isinstance(expr, ast.Name):
            name = expr.id
        elif isinstance(expr, ast.Attribute):
            name = expr.attr
        elif isinstance(expr, ast.Call):
            name = _call_name(expr.func)
        elif isinstance(expr, ast.Subscript):
            name = _leaf_name(expr.value)
        self.record(
            self.prediction_uses,
            name,
            getattr(expr, "lineno", 0),
            kind,
            detail,
        )

    def analyze(self):
        for statement in self.node.body:
            self.visit_statement(statement)

    def visit_statement(self, statement):
        if isinstance(statement, ast.Assign):
            kind = self.visit_expr(statement.value, False)
            for target in statement.targets:
                self.bind_target(target, kind)
            return
        if isinstance(statement, ast.AnnAssign):
            kind = self.visit_expr(statement.value, False) if statement.value is not None else "value"
            self.bind_target(statement.target, kind)
            return
        if isinstance(statement, ast.Expr):
            self.visit_expr(statement.value, False)
            return
        if isinstance(statement, ast.Return):
            if statement.value is not None:
                kind = self.visit_expr(statement.value, False)
                if kind == "prediction":
                    self.record_prediction_use(
                        statement.value,
                        "return_prediction",
                        "integration_test should thread model predictions into downstream surfaces instead of discarding them",
                    )
            return

        detail = "integration_test should stay declarative and should not contain control flow or other plain Python statements"
        self.record(
            self.illegal_body_logic,
            statement.__class__.__name__,
            getattr(statement, "lineno", 0),
            "statement",
            detail,
        )

    def bind_target(self, target, kind):
        if isinstance(target, ast.Name):
            self.allowed_values.add(target.id)
            if kind == "model":
                self.model_values.add(target.id)
            if kind.startswith("model:"):
                self.model_values.add(target.id)
                self.model_runtime_by_value[target.id] = kind.split(":", 1)[1]
            if kind == "model_metadata":
                self.model_metadata_values.add(target.id)
            if kind == "prediction":
                self.prediction_values.add(target.id)
            return
        if isinstance(target, (ast.Tuple, ast.List)):
            for item in target.elts:
                self.bind_target(item, kind)

    def visit_expr(self, expr, dataset_args_allowed):
        if expr is None:
            return "value"

        if isinstance(expr, ast.Call):
            return self.visit_call(expr)

        if isinstance(expr, ast.Name):
            if expr.id in {self.sample_arg, self.preprocess_arg} and not dataset_args_allowed:
                self.record(
                    self.direct_dataset_access,
                    expr.id,
                    getattr(expr, "lineno", 0),
                    "parameter_read",
                    "sample/preprocess arguments should only be forwarded into decorated Tensorleap calls",
                )
            if expr.id in self.model_values:
                return "model"
            if expr.id in self.model_metadata_values:
                return "model_metadata"
            if expr.id in self.prediction_values:
                return "prediction"
            return "value"

        if isinstance(expr, ast.Attribute):
            root = _root_name(expr)
            if root in {self.sample_arg, self.preprocess_arg}:
                self.record(
                    self.direct_dataset_access,
                    root,
                    getattr(expr, "lineno", 0),
                    "attribute_access",
                    "integration_test should not read fields from sample/preprocess objects directly",
                )
            else:
                base_kind = self.visit_expr(expr.value, False)
                if base_kind == "prediction":
                    self.record_prediction_use(
                        expr,
                        "prediction_attribute_access",
                        "integration_test reads attributes from model predictions",
                    )
                if base_kind == "model_metadata":
                    return "model_metadata"
            return "value"

        if isinstance(expr, ast.Subscript):
            base_kind = self.visit_expr(expr.value, False)
            self.visit_slice(expr.slice)
            if base_kind == "model_metadata":
                return "model_metadata"
            if base_kind != "prediction":
                self.record(
                    self.illegal_body_logic,
                    _leaf_name(expr.value),
                    getattr(expr, "lineno", 0),
                    "non_prediction_indexing",
                    "indexing is only allowed on model predictions inside integration_test",
                )
            else:
                self.record_prediction_use(
                    expr,
                    "prediction_indexing",
                    "integration_test indexes into model predictions",
                )
            if base_kind == "prediction":
                return "prediction"
            return "value"

        if isinstance(expr, (ast.List, ast.Tuple, ast.Set)):
            for item in expr.elts:
                self.visit_expr(item, False)
            return "value"

        if isinstance(expr, ast.Dict):
            for key in expr.keys:
                if key is not None:
                    self.visit_expr(key, False)
            for value in expr.values:
                self.visit_expr(value, False)
            return "value"

        if isinstance(expr, (ast.BinOp, ast.UnaryOp, ast.BoolOp, ast.Compare)):
            self.record(
                self.illegal_body_logic,
                expr.__class__.__name__,
                getattr(expr, "lineno", 0),
                "expression",
                "integration_test should not perform arithmetic or boolean logic; move that work into decorated interfaces",
            )
            saw_prediction = False
            for child in ast.iter_child_nodes(expr):
                if self.visit_expr(child, False) == "prediction":
                    saw_prediction = True
            if saw_prediction:
                self.record_prediction_use(
                    expr,
                    "prediction_expression",
                    "integration_test uses model predictions inside plain Python expressions",
                )
            return "value"

        if isinstance(expr, (ast.IfExp, ast.ListComp, ast.SetComp, ast.DictComp, ast.GeneratorExp, ast.Lambda, ast.NamedExpr, ast.JoinedStr, ast.FormattedValue)):
            self.record(
                self.illegal_body_logic,
                expr.__class__.__name__,
                getattr(expr, "lineno", 0),
                "expression",
                "integration_test should stay declarative; avoid inline Python transformations or formatting",
            )
            saw_prediction = False
            for child in ast.iter_child_nodes(expr):
                if self.visit_expr(child, False) == "prediction":
                    saw_prediction = True
            if saw_prediction:
                self.record_prediction_use(
                    expr,
                    "prediction_expression",
                    "integration_test uses model predictions inside inline Python logic",
                )
            return "value"

        for child in ast.iter_child_nodes(expr):
            self.visit_expr(child, False)
        return "value"

    def visit_slice(self, slice_node):
        if isinstance(slice_node, ast.Slice):
            for child in (slice_node.lower, slice_node.upper, slice_node.step):
                if child is not None:
                    self.visit_expr(child, False)
            return
        self.visit_expr(slice_node, False)

    def is_manual_batch_call(self, call):
        callee = _call_name(call.func).lower()
        if callee in MANUAL_BATCH_NAMES:
            return True
        if isinstance(call.func, ast.Attribute) and call.func.attr == "newaxis":
            return True
        for node in ast.walk(call):
            if isinstance(node, ast.Attribute) and node.attr == "newaxis":
                return True
        return False

    def is_transform_call(self, call):
        callee = _call_name(call.func).lower()
        if callee in TRANSFORM_NAMES:
            return True
        return _root_name(call.func) in LIBRARY_ROOTS

    def is_model_call(self, func):
        if isinstance(func, ast.Name):
            return func.id in self.model_values
        if isinstance(func, ast.Attribute):
            return _root_name(func) in self.model_values
        if isinstance(func, ast.Call):
            return _call_name(func.func) in self.load_model_functions
        return False

    def visit_call(self, call):
        callee_name = _call_name(call.func)
        root_name = _root_name(call.func)
        lower_callee = callee_name.lower()
        receiver_kind = ""

        if isinstance(call.func, ast.Attribute):
            receiver_kind = self.visit_expr(call.func.value, False)
            if receiver_kind == "prediction":
                self.record_prediction_use(
                    call,
                    "prediction_method_call",
                    "integration_test calls methods on model predictions",
                )

        if self.is_manual_batch_call(call):
            self.record(
                self.manual_batch,
                callee_name,
                getattr(call, "lineno", 0),
                "manual_batch",
                "Tensorleap adds the batch dimension automatically inside integration_test",
            )
            for arg in call.args:
                self.visit_expr(arg, False)
            for keyword in call.keywords:
                self.visit_expr(keyword.value, False)
            return "value"

        if callee_name in self.load_model_functions and not call.args and not call.keywords:
            self.calls.append(
                {
                    "name": callee_name,
                    "line": getattr(call, "lineno", 0),
                    "category": "decorated_load_model",
                }
            )
            runtime = self.load_model_runtime.get(callee_name, "")
            if runtime:
                return "model:" + runtime
            return "model"

        model_runtime = ""
        if isinstance(call.func, ast.Name):
            model_runtime = self.model_runtime_by_value.get(call.func.id, "")
        elif isinstance(call.func, ast.Attribute):
            model_runtime = self.model_runtime_by_value.get(_root_name(call.func), "")

        if model_runtime == "onnx_session":
            if isinstance(call.func, ast.Name):
                self.calls.append(
                    {
                        "name": callee_name or root_name,
                        "line": getattr(call, "lineno", 0),
                        "category": "model_call",
                    }
                )
                self.record(
                    self.illegal_body_logic,
                    callee_name or root_name,
                    getattr(call, "lineno", 0),
                    "invalid_model_call",
                    "integration_test calls a raw ONNX Runtime session as a callable; use model.run(...) with the session input mapping",
                )
                for arg in call.args:
                    self.visit_expr(arg, False)
                for keyword in call.keywords:
                    self.visit_expr(keyword.value, False)
                return "prediction"

            method = lower_callee
            if method in {"get_inputs", "get_outputs"}:
                self.calls.append(
                    {
                        "name": callee_name or root_name,
                        "line": getattr(call, "lineno", 0),
                        "category": "model_metadata",
                    }
                )
                for arg in call.args:
                    self.visit_expr(arg, False)
                for keyword in call.keywords:
                    self.visit_expr(keyword.value, False)
                return "model_metadata"

            if method == "run":
                self.calls.append(
                    {
                        "name": callee_name or root_name,
                        "line": getattr(call, "lineno", 0),
                        "category": "model_call",
                    }
                )
                for arg in call.args:
                    self.visit_expr(arg, False)
                for keyword in call.keywords:
                    self.visit_expr(keyword.value, False)
                return "prediction"

        if self.is_model_call(call.func):
            self.calls.append(
                {
                    "name": callee_name or root_name,
                    "line": getattr(call, "lineno", 0),
                    "category": "model_call",
                }
            )
            for arg in call.args:
                self.visit_expr(arg, False)
            for keyword in call.keywords:
                self.visit_expr(keyword.value, False)
            return "prediction"

        if callee_name in self.allowed_decorated_functions:
            decorated_kind = self.kind_by_function.get(callee_name, "decorated")
            self.calls.append(
                {
                    "name": callee_name,
                    "line": getattr(call, "lineno", 0),
                    "category": "decorated_" + decorated_kind,
                }
            )
            used_prediction = False
            for arg in call.args:
                if self.visit_expr(arg, True) == "prediction":
                    used_prediction = True
            for keyword in call.keywords:
                if self.visit_expr(keyword.value, True) == "prediction":
                    used_prediction = True
            if used_prediction:
                self.record_prediction_use(
                    call,
                    "prediction_argument",
                    "integration_test passes model predictions into a decorated interface",
                )
            return "decorator"

        if self.is_transform_call(call):
            self.record(
                self.illegal_body_logic,
                callee_name or root_name,
                getattr(call, "lineno", 0),
                "python_transform",
                "integration_test should not call external-library transforms directly; move that logic into decorated interfaces",
            )
        else:
            self.record(
                self.unknown_calls,
                callee_name or root_name,
                getattr(call, "lineno", 0),
                "unknown_call",
                "integration_test should only call Tensorleap decorators and the model inference path",
            )

        used_prediction = receiver_kind == "prediction"
        for arg in call.args:
            if self.visit_expr(arg, False) == "prediction":
                used_prediction = True
        for keyword in call.keywords:
            if self.visit_expr(keyword.value, False) == "prediction":
                used_prediction = True
        if used_prediction:
            self.record_prediction_use(
                call,
                "prediction_argument",
                "integration_test passes model predictions into a helper or transform call",
            )
        return "value"


repo_root = sys.argv[1]
entry_name = sys.argv[2]
target_path = repo_root + "/" + entry_name if not entry_name.startswith("/") else entry_name

with open(target_path, "r", encoding="utf-8") as handle:
    source = handle.read()

summary = {
    "available": True,
    "entryFile": entry_name,
}

try:
    tree = ast.parse(source, filename=entry_name)
except SyntaxError as exc:
    summary["parseError"] = str(exc.msg or "invalid Python syntax")
    summary["parseErrorLine"] = int(exc.lineno or 0)
    summary["parseErrorColumn"] = int(exc.offset or 0)
    print(json.dumps(summary, sort_keys=True))
    sys.exit(0)

decorated_functions, integration_tests, kind_by_function = _decorated_functions(tree)
summary["decoratedFunctions"] = decorated_functions

analyzed_tests = []
for function_node in integration_tests:
    analyzer = _IntegrationFunctionAnalyzer(function_node, decorated_functions, kind_by_function)
    analyzer.analyze()
    analyzed_tests.append(analyzer.result())

summary["integrationTests"] = analyzed_tests
print(json.dumps(summary, sort_keys=True))
`

// IntegrationTestASTAnalyzer runs a Python-stdlib AST check for integration-test wiring.
type IntegrationTestASTAnalyzer struct {
	runtimeRunner guideRuntimeRunner
}

// IntegrationTestASTResult captures static integration-test findings.
type IntegrationTestASTResult struct {
	Summary  IntegrationTestASTSummary
	Issues   []core.Issue
	Evidence []core.EvidenceItem
}

// IntegrationTestASTSummary is the machine-readable output of the integration-test AST pass.
type IntegrationTestASTSummary struct {
	Available          bool                               `json:"available,omitempty"`
	UnavailableReason  string                             `json:"unavailableReason,omitempty"`
	EntryFile          string                             `json:"entryFile,omitempty"`
	ParseError         string                             `json:"parseError,omitempty"`
	ParseErrorLine     int                                `json:"parseErrorLine,omitempty"`
	ParseErrorColumn   int                                `json:"parseErrorColumn,omitempty"`
	DecoratedFunctions []IntegrationTestDecoratedFunction `json:"decoratedFunctions,omitempty"`
	IntegrationTests   []IntegrationTestFunctionAnalysis  `json:"integrationTests,omitempty"`
}

// IntegrationTestDecoratedFunction captures one Tensorleap-decorated function discovered in source.
type IntegrationTestDecoratedFunction struct {
	Function string `json:"function,omitempty"`
	Kind     string `json:"kind,omitempty"`
	Runtime  string `json:"runtime,omitempty"`
	Symbol   string `json:"symbol,omitempty"`
	Line     int    `json:"line,omitempty"`
}

// IntegrationTestFunctionAnalysis captures one analyzed @tensorleap_integration_test body.
type IntegrationTestFunctionAnalysis struct {
	Function                 string                      `json:"function,omitempty"`
	Line                     int                         `json:"line,omitempty"`
	Arguments                []string                    `json:"arguments,omitempty"`
	Calls                    []IntegrationTestCall       `json:"calls,omitempty"`
	PredictionUses           []IntegrationTestASTFinding `json:"predictionUses,omitempty"`
	UnknownCalls             []IntegrationTestASTFinding `json:"unknownCalls,omitempty"`
	DirectDatasetAccess      []IntegrationTestASTFinding `json:"directDatasetAccess,omitempty"`
	ManualBatchManipulations []IntegrationTestASTFinding `json:"manualBatchManipulations,omitempty"`
	IllegalBodyLogic         []IntegrationTestASTFinding `json:"illegalBodyLogic,omitempty"`
}

// IntegrationTestCall captures one function call inside the integration test body.
type IntegrationTestCall struct {
	Name     string `json:"name,omitempty"`
	Line     int    `json:"line,omitempty"`
	Category string `json:"category,omitempty"`
}

// IntegrationTestASTFinding captures one static violation in the integration test body.
type IntegrationTestASTFinding struct {
	Name   string `json:"name,omitempty"`
	Line   int    `json:"line,omitempty"`
	Kind   string `json:"kind,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// NewIntegrationTestASTAnalyzer creates an analyzer backed by the resolved Poetry runtime.
func NewIntegrationTestASTAnalyzer() *IntegrationTestASTAnalyzer {
	return &IntegrationTestASTAnalyzer{runtimeRunner: NewPythonRuntimeRunner()}
}

// Analyze inspects leap_integration.py using Python's stdlib AST module.
func (a *IntegrationTestASTAnalyzer) Analyze(ctx context.Context, snapshot core.WorkspaceSnapshot) (IntegrationTestASTResult, error) {
	if a == nil {
		a = NewIntegrationTestASTAnalyzer()
	}
	if a.runtimeRunner == nil {
		a.runtimeRunner = NewPythonRuntimeRunner()
	}

	summary := IntegrationTestASTSummary{}
	skipReason, entryName, err := guideValidationSkipReason(snapshot)
	if err != nil {
		return IntegrationTestASTResult{}, err
	}
	if skipReason != "" {
		summary.UnavailableReason = skipReason
		return IntegrationTestASTResult{
			Summary: summary,
			Evidence: []core.EvidenceItem{
				marshalGuideEvidence(integrationTestASTEvidenceSummary, summary),
			},
		}, nil
	}

	repoRoot := strings.TrimSpace(snapshot.Repository.Root)
	result, err := a.runtimeRunner.RunPython(ctx, snapshot, "-c", integrationTestASTScript, repoRoot, entryName)
	evidence := []core.EvidenceItem{
		{Name: integrationTestASTEvidenceCommand, Value: result.Command},
		{Name: integrationTestASTEvidenceStdout, Value: result.Stdout},
		{Name: integrationTestASTEvidenceStderr, Value: result.Stderr},
	}
	if err != nil {
		return IntegrationTestASTResult{}, core.WrapError(core.KindUnknown, "validate.integration_test_ast.run", err)
	}

	if strings.TrimSpace(result.Stdout) != "" {
		if err := json.Unmarshal([]byte(result.Stdout), &summary); err != nil {
			return IntegrationTestASTResult{}, core.WrapError(core.KindUnknown, "validate.integration_test_ast.parse", err)
		}
	}
	summary.EntryFile = entryName
	issues := integrationTestASTIssues(summary, snapshot)
	evidence = append(evidence, marshalGuideEvidence(integrationTestASTEvidenceSummary, summary))

	return IntegrationTestASTResult{
		Summary:  summary,
		Issues:   dedupeIssues(issues),
		Evidence: evidence,
	}, nil
}

func integrationTestASTIssues(summary IntegrationTestASTSummary, snapshot core.WorkspaceSnapshot) []core.Issue {
	if !summary.Available && strings.TrimSpace(summary.UnavailableReason) != "" {
		return nil
	}

	issues := make([]core.Issue, 0, 8)
	entryFile := strings.TrimSpace(summary.EntryFile)
	if entryFile == "" {
		entryFile = core.CanonicalIntegrationEntryFile
	}

	if strings.TrimSpace(summary.ParseError) != "" {
		location := &core.IssueLocation{Path: entryFile}
		if summary.ParseErrorLine > 0 {
			location.Line = summary.ParseErrorLine
		}
		if summary.ParseErrorColumn > 0 {
			location.Column = summary.ParseErrorColumn
		}
		issues = append(issues, core.Issue{
			Code:     core.IssueCodeIntegrationScriptImportFailed,
			Message:  fmt.Sprintf("integration_test analysis could not parse %s: %s", entryFile, strings.TrimSpace(summary.ParseError)),
			Severity: core.SeverityError,
			Scope:    core.IssueScopeIntegrationScript,
			Location: location,
		})
		return issues
	}

	if len(summary.IntegrationTests) == 0 {
		return nil
	}

	calledFunctions := calledIntegrationFunctions(summary.IntegrationTests)
	integrationLine := firstIntegrationTestLine(summary.IntegrationTests)
	locationFor := func(symbol string, line int) *core.IssueLocation {
		location := &core.IssueLocation{
			Path:   entryFile,
			Symbol: strings.TrimSpace(symbol),
		}
		if line > 0 {
			location.Line = line
		} else if integrationLine > 0 {
			location.Line = integrationLine
		}
		return location
	}

	for _, function := range requiredIntegrationTestFunctions(summary, snapshot, "input_encoder") {
		if _, ok := calledFunctions[strings.ToLower(function.Function)]; ok {
			continue
		}
		target := strings.TrimSpace(function.Symbol)
		if target == "" {
			target = strings.TrimSpace(function.Function)
		}
		issues = append(issues, core.Issue{
			Code:     core.IssueCodeIntegrationTestMissingRequiredCalls,
			Message:  fmt.Sprintf("integration_test does not call the decorated input encoder for required input name %q", target),
			Severity: core.SeverityError,
			Scope:    core.IssueScopeIntegrationTest,
			Location: locationFor(target, integrationLine),
		})
	}

	for _, function := range requiredIntegrationTestFunctions(summary, snapshot, "load_model") {
		if _, ok := calledFunctions[strings.ToLower(function.Function)]; ok {
			continue
		}
		target := strings.TrimSpace(function.Function)
		issues = append(issues, core.Issue{
			Code:     core.IssueCodeIntegrationTestMissingRequiredCalls,
			Message:  fmt.Sprintf("integration_test does not call the decorated load_model function %q", target),
			Severity: core.SeverityError,
			Scope:    core.IssueScopeIntegrationTest,
			Location: locationFor(target, integrationLine),
		})
	}

	for _, function := range requiredIntegrationTestFunctions(summary, snapshot, "gt_encoder") {
		if _, ok := calledFunctions[strings.ToLower(function.Function)]; ok {
			continue
		}
		target := strings.TrimSpace(function.Symbol)
		if target == "" {
			target = strings.TrimSpace(function.Function)
		}
		issues = append(issues, core.Issue{
			Code:     core.IssueCodeIntegrationTestMissingRequiredCalls,
			Message:  fmt.Sprintf("integration_test does not call the decorated ground-truth encoder for required ground-truth name %q", target),
			Severity: core.SeverityError,
			Scope:    core.IssueScopeIntegrationTest,
			Location: locationFor(target, integrationLine),
		})
	}

	requiredLoadModels := requiredIntegrationTestFunctions(summary, snapshot, "load_model")
	for _, analysis := range summary.IntegrationTests {
		modelCallLine := 0
		calledRequiredLoadModel := false
		for _, call := range analysis.Calls {
			if call.Category == "model_call" && modelCallLine == 0 {
				modelCallLine = call.Line
			}
			if calledRequiredLoadModel {
				continue
			}
			for _, function := range requiredLoadModels {
				if strings.EqualFold(call.Name, function.Function) {
					calledRequiredLoadModel = true
					break
				}
			}
		}
		if len(requiredLoadModels) > 0 && calledRequiredLoadModel && modelCallLine == 0 {
			issues = append(issues, core.Issue{
				Code:     core.IssueCodeIntegrationTestMissingRequiredCalls,
				Message:  "integration_test calls load_model() but never executes model inference",
				Severity: core.SeverityError,
				Scope:    core.IssueScopeIntegrationTest,
				Location: locationFor("model_inference", analysis.Line),
			})
		}
		if modelCallLine > 0 && len(analysis.PredictionUses) == 0 {
			issues = append(issues, core.Issue{
				Code:     core.IssueCodeIntegrationTestMissingRequiredCalls,
				Message:  "integration_test executes model inference but never consumes model outputs",
				Severity: core.SeverityError,
				Scope:    core.IssueScopeIntegrationTest,
				Location: locationFor("prediction_outputs", modelCallLine),
			})
		}
		for _, finding := range analysis.UnknownCalls {
			target := strings.TrimSpace(finding.Name)
			if target == "" {
				target = strings.TrimSpace(analysis.Function)
			}
			issues = append(issues, core.Issue{
				Code:     core.IssueCodeIntegrationTestCallsUnknownInterfaces,
				Message:  fmt.Sprintf("integration_test calls non-decorated helper %q; keep integration_test limited to Tensorleap decorators and model inference", target),
				Severity: core.SeverityError,
				Scope:    core.IssueScopeIntegrationTest,
				Location: locationFor(target, finding.Line),
			})
		}
		for _, finding := range analysis.DirectDatasetAccess {
			target := strings.TrimSpace(finding.Name)
			if target == "" {
				target = strings.TrimSpace(analysis.Function)
			}
			message := strings.TrimSpace(finding.Detail)
			if message == "" {
				message = "integration_test should not read sample/preprocess objects directly; move that logic into decorated interfaces"
			}
			issues = append(issues, core.Issue{
				Code:     core.IssueCodeIntegrationTestDirectDatasetAccess,
				Message:  message,
				Severity: core.SeverityError,
				Scope:    core.IssueScopeIntegrationTest,
				Location: locationFor(target, finding.Line),
			})
		}
		for _, finding := range analysis.ManualBatchManipulations {
			target := strings.TrimSpace(finding.Name)
			message := strings.TrimSpace(finding.Detail)
			if message == "" {
				message = "integration_test manually adds a batch dimension; Tensorleap handles batching automatically"
			}
			issues = append(issues, core.Issue{
				Code:     core.IssueCodeIntegrationTestManualBatchManipulation,
				Message:  message,
				Severity: core.SeverityError,
				Scope:    core.IssueScopeIntegrationTest,
				Location: locationFor(target, finding.Line),
			})
		}
		for _, finding := range analysis.IllegalBodyLogic {
			target := strings.TrimSpace(finding.Name)
			message := strings.TrimSpace(finding.Detail)
			if message == "" {
				message = "integration_test contains ordinary Python logic; keep it thin and declarative"
			}
			issues = append(issues, core.Issue{
				Code:     core.IssueCodeIntegrationTestIllegalBodyLogic,
				Message:  message,
				Severity: core.SeverityError,
				Scope:    core.IssueScopeIntegrationTest,
				Location: locationFor(target, finding.Line),
			})
		}
	}

	return issues
}

func requiredIntegrationTestFunctions(
	summary IntegrationTestASTSummary,
	snapshot core.WorkspaceSnapshot,
	kind string,
) []IntegrationTestDecoratedFunction {
	all := make([]IntegrationTestDecoratedFunction, 0, len(summary.DecoratedFunctions))
	for _, function := range summary.DecoratedFunctions {
		if strings.TrimSpace(function.Kind) != kind {
			continue
		}
		all = append(all, function)
	}

	switch kind {
	case "input_encoder":
		symbols := mappingSymbols(snapshot.ConfirmedEncoderMapping, true)
		if len(symbols) == 0 {
			return dedupeDecoratedFunctions(all)
		}
		filtered := make([]IntegrationTestDecoratedFunction, 0, len(all))
		for _, function := range all {
			if _, ok := symbols[strings.ToLower(strings.TrimSpace(function.Symbol))]; !ok {
				continue
			}
			filtered = append(filtered, function)
		}
		return dedupeDecoratedFunctions(filtered)
	case "gt_encoder":
		symbols := mappingSymbols(snapshot.ConfirmedEncoderMapping, false)
		if len(symbols) == 0 {
			return dedupeDecoratedFunctions(all)
		}
		filtered := make([]IntegrationTestDecoratedFunction, 0, len(all))
		for _, function := range all {
			if _, ok := symbols[strings.ToLower(strings.TrimSpace(function.Symbol))]; !ok {
				continue
			}
			filtered = append(filtered, function)
		}
		return dedupeDecoratedFunctions(filtered)
	default:
		return dedupeDecoratedFunctions(all)
	}
}

func mappingSymbols(mapping *core.EncoderMappingContract, input bool) map[string]struct{} {
	if mapping == nil {
		return nil
	}
	values := mapping.GroundTruthSymbols
	if input {
		values = mapping.InputSymbols
	}
	if len(values) == 0 {
		return nil
	}

	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		key := strings.ToLower(strings.TrimSpace(value))
		if key == "" {
			continue
		}
		set[key] = struct{}{}
	}
	if len(set) == 0 {
		return nil
	}
	return set
}

func dedupeDecoratedFunctions(values []IntegrationTestDecoratedFunction) []IntegrationTestDecoratedFunction {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]IntegrationTestDecoratedFunction, len(values))
	for _, value := range values {
		key := strings.ToLower(strings.TrimSpace(value.Kind)) + "|" + strings.ToLower(strings.TrimSpace(value.Function)) + "|" + strings.ToLower(strings.TrimSpace(value.Symbol))
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = value
	}

	unique := make([]IntegrationTestDecoratedFunction, 0, len(seen))
	for _, value := range seen {
		unique = append(unique, value)
	}
	sort.Slice(unique, func(i, j int) bool {
		if unique[i].Kind != unique[j].Kind {
			return unique[i].Kind < unique[j].Kind
		}
		if unique[i].Symbol != unique[j].Symbol {
			return unique[i].Symbol < unique[j].Symbol
		}
		return unique[i].Function < unique[j].Function
	})
	return unique
}

func calledIntegrationFunctions(tests []IntegrationTestFunctionAnalysis) map[string]IntegrationTestCall {
	calls := make(map[string]IntegrationTestCall)
	for _, test := range tests {
		for _, call := range test.Calls {
			key := strings.ToLower(strings.TrimSpace(call.Name))
			if key == "" {
				continue
			}
			if _, ok := calls[key]; ok {
				continue
			}
			calls[key] = call
		}
	}
	return calls
}

func firstIntegrationTestLine(tests []IntegrationTestFunctionAnalysis) int {
	for _, test := range tests {
		if test.Line > 0 {
			return test.Line
		}
	}
	return 0
}

from __future__ import annotations

import ast
import json
from pathlib import Path
from typing import Any, Iterable, Mapping

SCHEMA_VERSION = 1
REVIEW_FILE_PATHS = ("leap.yaml", "leap_integration.py", "leap_binder.py", "leap_custom_test.py")
REVIEW_PYTHON_PATHS = tuple(path for path in REVIEW_FILE_PATHS if path.endswith(".py"))
PRIMARY_CRITERIA_BY_STEP = {
    "pre": [
        "checkpoint_appropriateness",
        "encoder_inventory_alignment",
        "prediction_surface_alignment",
        "preprocess_sample_model_alignment",
        "input_tensor_contract_alignment",
        "gt_contract_alignment",
    ],
    "preprocess": [
        "preprocess_sample_model_alignment",
        "checkpoint_appropriateness",
        "encoder_inventory_alignment",
        "prediction_surface_alignment",
        "input_tensor_contract_alignment",
        "gt_contract_alignment",
    ],
    "input_encoders": [
        "encoder_inventory_alignment",
        "input_tensor_contract_alignment",
        "preprocess_sample_model_alignment",
        "prediction_surface_alignment",
        "checkpoint_appropriateness",
        "gt_contract_alignment",
    ],
    "model_acquisition": [
        "prediction_surface_alignment",
        "preprocess_sample_model_alignment",
        "checkpoint_appropriateness",
        "encoder_inventory_alignment",
        "input_tensor_contract_alignment",
        "gt_contract_alignment",
    ],
    "integration_test": [
        "preprocess_sample_model_alignment",
        "prediction_surface_alignment",
        "encoder_inventory_alignment",
        "input_tensor_contract_alignment",
        "gt_contract_alignment",
        "checkpoint_appropriateness",
    ],
    "ground_truth_encoders": [
        "gt_contract_alignment",
        "encoder_inventory_alignment",
        "preprocess_sample_model_alignment",
        "prediction_surface_alignment",
        "input_tensor_contract_alignment",
        "checkpoint_appropriateness",
    ],
}
STEP_NOTES = {
    "pre": "Use the post fixture as the ground-truth ceiling, but treat missing downstream surfaces as checkpoint-appropriateness evidence rather than automatic literal mismatches.",
    "preprocess": "Preprocess and sample-id surfaces are primary; downstream encoder and model deltas are supporting evidence.",
    "input_encoders": "Input registrations, channel dimensions, and integration-test forwarding are primary review surfaces.",
    "model_acquisition": "Model-loading and prediction-surface facts are primary; encoder inventories are supporting evidence.",
    "integration_test": "The integration-test call shape and prediction consumption are primary review surfaces.",
    "ground_truth_encoders": "Ground-truth registrations and GT call usage are primary review surfaces.",
}


def build_final_review_comparison_bundle(
    *,
    run_context: Mapping[str, Any],
    candidate_workspace: Path,
    fixture_post_path: Path,
) -> dict[str, Any]:
    guide_step = clean_text(run_context.get("guide_step", ""))
    candidate = analyze_review_root(candidate_workspace)
    fixture_post = analyze_review_root(fixture_post_path)
    return {
        "schema_version": SCHEMA_VERSION,
        "run": {
            "run_id": clean_text(run_context.get("run_id", "")),
            "fixture_id": clean_text(run_context.get("fixture_id", "")),
            "guide_step": guide_step,
            "ref_under_test": clean_text(run_context.get("ref_under_test", "")),
            "checkpoint_key": clean_text(run_context.get("checkpoint_key", "")),
            "source_kind": clean_text(run_context.get("source_kind", "")),
            "source_id": clean_text(run_context.get("source_id", "")),
            "stop_reason": clean_text(run_context.get("stop_reason", "")),
        },
        "step_context": {
            "primary_criteria": primary_criteria_for_step(guide_step),
            "note": STEP_NOTES.get(guide_step, "Review the extracted comparison surfaces with the active checkpoint in mind."),
        },
        "candidate": candidate,
        "fixture_post": fixture_post,
        "comparison": build_comparison(candidate, fixture_post),
    }


def write_final_review_comparison_bundle(bundle: Mapping[str, Any], path: Path) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(bundle, indent=2, sort_keys=True) + "\n", encoding="utf-8")


def primary_criteria_for_step(guide_step: str) -> list[str]:
    if guide_step in PRIMARY_CRITERIA_BY_STEP:
        return list(PRIMARY_CRITERIA_BY_STEP[guide_step])
    return list(PRIMARY_CRITERIA_BY_STEP["pre"])


def analyze_review_root(root: Path) -> dict[str, Any]:
    files_present = [relative_path for relative_path in REVIEW_FILE_PATHS if (root / relative_path).exists()]
    python_analysis = {
        "top_level_functions": [],
        "decorated_functions": {},
        "preprocess_functions": [],
        "input_encoders": [],
        "gt_encoders": [],
        "load_model_functions": [],
        "integration_tests": [],
        "prediction_types": [],
        "main_blocks": [],
        "parse_errors": [],
    }

    for relative_path in REVIEW_PYTHON_PATHS:
        path = root / relative_path
        if not path.is_file():
            continue
        analysis = analyze_python_file(path, relative_path)
        python_analysis["top_level_functions"].extend(analysis["top_level_functions"])
        python_analysis["preprocess_functions"].extend(analysis["preprocess_functions"])
        python_analysis["input_encoders"].extend(analysis["input_encoders"])
        python_analysis["gt_encoders"].extend(analysis["gt_encoders"])
        python_analysis["load_model_functions"].extend(analysis["load_model_functions"])
        python_analysis["integration_tests"].extend(analysis["integration_tests"])
        python_analysis["prediction_types"].extend(analysis["prediction_types"])
        python_analysis["main_blocks"].extend(analysis["main_blocks"])
        python_analysis["parse_errors"].extend(analysis["parse_errors"])
        merge_decorated_functions(python_analysis["decorated_functions"], analysis["decorated_functions"])

    sort_python_analysis(python_analysis)
    return {
        "files_present": files_present,
        "leap_yaml": parse_leap_yaml(root / "leap.yaml"),
        "python": python_analysis,
    }


def merge_decorated_functions(target: dict[str, list[dict[str, Any]]], source: Mapping[str, Iterable[dict[str, Any]]]) -> None:
    for decorator_name, entries in source.items():
        target.setdefault(decorator_name, []).extend(entries)


def sort_python_analysis(analysis: dict[str, Any]) -> None:
    for key in (
        "top_level_functions",
        "preprocess_functions",
        "input_encoders",
        "gt_encoders",
        "load_model_functions",
        "integration_tests",
        "prediction_types",
        "main_blocks",
        "parse_errors",
    ):
        analysis[key] = sorted(analysis[key], key=sort_key)
    analysis["decorated_functions"] = {
        key: sorted(value, key=sort_key)
        for key, value in sorted(analysis["decorated_functions"].items(), key=lambda item: item[0])
    }


def parse_leap_yaml(path: Path) -> dict[str, Any]:
    if not path.is_file():
        return {"present": False}

    entry_file = ""
    include: list[str] = []
    current_list_key = ""
    for raw_line in path.read_text(encoding="utf-8").splitlines():
        line = raw_line.rstrip()
        stripped = line.strip()
        if not stripped or stripped.startswith("#"):
            continue
        if not line.startswith((" ", "\t")):
            current_list_key = ""
            if ":" in stripped:
                key, value = stripped.split(":", 1)
                key = key.strip()
                value = value.strip().strip('"').strip("'")
                if key == "entryFile":
                    entry_file = value
                elif key == "include" and not value:
                    current_list_key = "include"
            continue
        if current_list_key == "include" and stripped.startswith("- "):
            include.append(stripped[2:].strip().strip('"').strip("'"))
    return {
        "present": True,
        "path": "leap.yaml",
        "entry_file": entry_file,
        "include": include,
    }


def analyze_python_file(path: Path, relative_path: str) -> dict[str, Any]:
    source = path.read_text(encoding="utf-8")
    lines = source.splitlines()
    try:
        tree = ast.parse(source, filename=relative_path)
    except SyntaxError as exc:
        return {
            "top_level_functions": [],
            "decorated_functions": {},
            "preprocess_functions": [],
            "input_encoders": [],
            "gt_encoders": [],
            "load_model_functions": [],
            "integration_tests": [],
            "prediction_types": [],
            "main_blocks": [],
            "parse_errors": [
                {
                    "path": relative_path,
                    "message": f"{exc.msg} (line {exc.lineno})",
                }
            ],
        }

    top_level_functions: list[dict[str, Any]] = []
    decorated_functions: dict[str, list[dict[str, Any]]] = {}
    preprocess_functions: list[dict[str, Any]] = []
    input_encoders: list[dict[str, Any]] = []
    gt_encoders: list[dict[str, Any]] = []
    load_model_functions: list[dict[str, Any]] = []
    integration_tests: list[dict[str, Any]] = []
    prediction_types: list[dict[str, Any]] = []
    main_blocks: list[dict[str, Any]] = []

    for node in tree.body:
        if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)):
            function_summary = build_function_summary(node, source, lines, relative_path)
            top_level_functions.append(function_summary)
            for decorator in function_summary["decorators"]:
                decorated_functions.setdefault(decorator["name"], []).append(
                    {
                        "function": function_summary["name"],
                        "args": list(function_summary["args"]),
                        "registered_name": decorator.get("registered_name"),
                        "channel_dim": decorator.get("channel_dim"),
                        "prediction_type_refs": list(decorator.get("prediction_type_refs", [])),
                        "provenance": function_summary["provenance"],
                    }
                )
                if decorator["name"] == "tensorleap_preprocess":
                    preprocess_functions.append(
                        {
                            "function": function_summary["name"],
                            "args": list(function_summary["args"]),
                            "provenance": function_summary["provenance"],
                        }
                    )
                elif decorator["name"] == "tensorleap_input_encoder":
                    input_encoders.append(
                        {
                            "function": function_summary["name"],
                            "name": clean_text(decorator.get("registered_name", "")),
                            "channel_dim": decorator.get("channel_dim"),
                            "args": list(function_summary["args"]),
                            "provenance": function_summary["provenance"],
                        }
                    )
                elif decorator["name"] == "tensorleap_gt_encoder":
                    gt_encoders.append(
                        {
                            "function": function_summary["name"],
                            "name": clean_text(decorator.get("registered_name", "")),
                            "run_on_unlabeled": decorator.get("run_on_unlabeled"),
                            "args": list(function_summary["args"]),
                            "provenance": function_summary["provenance"],
                        }
                    )
                elif decorator["name"] == "tensorleap_load_model":
                    load_model_functions.append(
                        {
                            "function": function_summary["name"],
                            "prediction_type_refs": list(decorator.get("prediction_type_refs", [])),
                            "args": list(function_summary["args"]),
                            "provenance": function_summary["provenance"],
                        }
                    )
                elif decorator["name"] == "tensorleap_integration_test":
                    integration_tests.append(
                        {
                            "function": function_summary["name"],
                            "args": list(function_summary["args"]),
                            "sample_id_arg": function_summary["args"][0] if function_summary["args"] else "",
                            "preprocess_arg": function_summary["args"][1] if len(function_summary["args"]) > 1 else "",
                            "call_sequence": collect_call_sequence(node.body, source, lines, relative_path),
                            "provenance": function_summary["provenance"],
                        }
                    )
        elif isinstance(node, ast.Assign):
            prediction_type = extract_prediction_type(node, source, lines, relative_path)
            if prediction_type is not None:
                prediction_types.append(prediction_type)
        elif is_main_block(node):
            main_blocks.append(
                {
                    "call_sequence": collect_call_sequence(node.body, source, lines, relative_path),
                    "sample_id_loops": collect_sample_id_loops(node.body, source, lines, relative_path),
                    "provenance": provenance_for_node(node, lines, relative_path),
                }
            )

    return {
        "top_level_functions": top_level_functions,
        "decorated_functions": decorated_functions,
        "preprocess_functions": preprocess_functions,
        "input_encoders": input_encoders,
        "gt_encoders": gt_encoders,
        "load_model_functions": load_model_functions,
        "integration_tests": integration_tests,
        "prediction_types": prediction_types,
        "main_blocks": main_blocks,
        "parse_errors": [],
    }


def build_function_summary(
    node: ast.FunctionDef | ast.AsyncFunctionDef,
    source: str,
    lines: list[str],
    relative_path: str,
) -> dict[str, Any]:
    return {
        "name": node.name,
        "path": relative_path,
        "args": [argument.arg for argument in node.args.args],
        "decorators": [
            extract_decorator_summary(decorator, source, lines, relative_path)
            for decorator in node.decorator_list
        ],
        "provenance": provenance_for_function(node, lines, relative_path),
    }


def extract_decorator_summary(
    decorator: ast.expr,
    source: str,
    lines: list[str],
    relative_path: str,
) -> dict[str, Any]:
    call = decorator if isinstance(decorator, ast.Call) else None
    name = normalized_name(call.func if call is not None else decorator)
    registered_name = ""
    channel_dim: int | None = None
    run_on_unlabeled: bool | None = None
    prediction_type_refs: list[str] = []
    if call is not None:
        if name in {"tensorleap_input_encoder", "tensorleap_gt_encoder"}:
            registered_name = extract_string_argument(call, keyword_name="name")
        if name == "tensorleap_input_encoder":
            channel_dim = extract_int_keyword(call, "channel_dim")
        if name == "tensorleap_gt_encoder":
            run_on_unlabeled = extract_bool_keyword(call, "run_on_unlabeled")
        if name == "tensorleap_load_model":
            prediction_type_refs = extract_prediction_type_refs(call, source)
    return {
        "name": name,
        "registered_name": registered_name,
        "channel_dim": channel_dim,
        "run_on_unlabeled": run_on_unlabeled,
        "prediction_type_refs": prediction_type_refs,
        "provenance": provenance_for_node(decorator, lines, relative_path),
    }


def extract_prediction_type(node: ast.Assign, source: str, lines: list[str], relative_path: str) -> dict[str, Any] | None:
    if not isinstance(node.value, ast.Call):
        return None
    if normalized_name(node.value.func) != "PredictionTypeHandler":
        return None
    variable = ""
    for target in node.targets:
        if isinstance(target, ast.Name):
            variable = target.id
            break
    labels_node = extract_keyword_value(node.value, "labels")
    return {
        "variable": variable,
        "name": extract_string_argument(node.value, keyword_name="name"),
        "channel_dim": extract_int_keyword(node.value, "channel_dim"),
        "labels_expression": render_node(labels_node, source),
        "label_count_hint": literal_sequence_length(labels_node),
        "provenance": provenance_for_node(node, lines, relative_path),
    }


def collect_call_sequence(
    statements: Iterable[ast.stmt],
    source: str,
    lines: list[str],
    relative_path: str,
) -> list[dict[str, Any]]:
    collector = OrderedCallCollector(source=source, lines=lines, relative_path=relative_path)
    for statement in statements:
        collector.visit(statement)
    return collector.calls


def collect_sample_id_loops(
    statements: Iterable[ast.stmt],
    source: str,
    lines: list[str],
    relative_path: str,
) -> list[dict[str, Any]]:
    collector = SampleIDLoopCollector(source=source, lines=lines, relative_path=relative_path)
    for statement in statements:
        collector.visit(statement)
    return collector.loops


class OrderedCallCollector(ast.NodeVisitor):
    def __init__(self, *, source: str, lines: list[str], relative_path: str) -> None:
        self.source = source
        self.lines = lines
        self.relative_path = relative_path
        self.calls: list[dict[str, Any]] = []

    def visit_Call(self, node: ast.Call) -> None:
        self.calls.append(
            {
                "callee": normalized_name(node.func),
                "arguments": [render_node(argument, self.source) for argument in node.args],
                "keywords": {
                    clean_text(keyword.arg): render_node(keyword.value, self.source)
                    for keyword in node.keywords
                    if keyword.arg is not None
                },
                "provenance": provenance_for_node(node, self.lines, self.relative_path, max_lines=3),
            }
        )
        self.generic_visit(node)


class SampleIDLoopCollector(ast.NodeVisitor):
    def __init__(self, *, source: str, lines: list[str], relative_path: str) -> None:
        self.source = source
        self.lines = lines
        self.relative_path = relative_path
        self.loops: list[dict[str, Any]] = []

    def visit_For(self, node: ast.For) -> None:
        iter_expression = render_node(node.iter, self.source)
        if ".sample_ids" in iter_expression:
            self.loops.append(
                {
                    "target": render_node(node.target, self.source),
                    "iter_expression": iter_expression,
                    "provenance": provenance_for_node(node, self.lines, self.relative_path),
                }
            )
        self.generic_visit(node)


def build_comparison(candidate: Mapping[str, Any], fixture_post: Mapping[str, Any]) -> dict[str, Any]:
    candidate_python = candidate["python"]
    fixture_python = fixture_post["python"]
    return {
        "function_inventory": compare_name_inventory(
            candidate_python["top_level_functions"],
            fixture_python["top_level_functions"],
            item_name=lambda item: clean_text(item.get("name", "")),
        ),
        "decorator_inventory": compare_decorator_inventory(
            candidate_python["decorated_functions"],
            fixture_python["decorated_functions"],
        ),
        "input_encoders": compare_name_inventory(
            candidate_python["input_encoders"],
            fixture_python["input_encoders"],
            item_name=lambda item: clean_text(item.get("name", "")),
            extra_fields=("channel_dim",),
        ),
        "gt_encoders": compare_name_inventory(
            candidate_python["gt_encoders"],
            fixture_python["gt_encoders"],
            item_name=lambda item: clean_text(item.get("name", "")),
        ),
        "prediction_types": compare_name_inventory(
            candidate_python["prediction_types"],
            fixture_python["prediction_types"],
            item_name=lambda item: clean_text(item.get("name", "")) or clean_text(item.get("variable", "")),
            extra_fields=("channel_dim",),
        ),
        "integration_tests": compare_call_sequences(
            candidate_python["integration_tests"],
            fixture_python["integration_tests"],
        ),
        "preprocess_surface": {
            "candidate_functions": sorted(
                clean_text(item.get("function", "")) for item in candidate_python["preprocess_functions"] if clean_text(item.get("function", ""))
            ),
            "fixture_functions": sorted(
                clean_text(item.get("function", "")) for item in fixture_python["preprocess_functions"] if clean_text(item.get("function", ""))
            ),
            "candidate_main_sample_id_loops": [
                clean_text(loop.get("iter_expression", ""))
                for block in candidate_python["main_blocks"]
                for loop in block.get("sample_id_loops", [])
                if clean_text(loop.get("iter_expression", ""))
            ],
            "fixture_main_sample_id_loops": [
                clean_text(loop.get("iter_expression", ""))
                for block in fixture_python["main_blocks"]
                for loop in block.get("sample_id_loops", [])
                if clean_text(loop.get("iter_expression", ""))
            ],
        },
    }


def compare_decorator_inventory(
    candidate: Mapping[str, Iterable[dict[str, Any]]],
    fixture_post: Mapping[str, Iterable[dict[str, Any]]],
) -> dict[str, Any]:
    candidate_counts = {
        decorator: len(list(entries))
        for decorator, entries in sorted(candidate.items(), key=lambda item: item[0])
    }
    fixture_counts = {
        decorator: len(list(entries))
        for decorator, entries in sorted(fixture_post.items(), key=lambda item: item[0])
    }
    candidate_names = sorted(candidate_counts)
    fixture_names = sorted(fixture_counts)
    return {
        "candidate_counts": candidate_counts,
        "fixture_counts": fixture_counts,
        "candidate_only": sorted(name for name in candidate_names if name not in fixture_counts),
        "fixture_only": sorted(name for name in fixture_names if name not in candidate_counts),
        "shared": sorted(name for name in candidate_names if name in fixture_counts),
    }


def compare_name_inventory(
    candidate: Iterable[Mapping[str, Any]],
    fixture_post: Iterable[Mapping[str, Any]],
    *,
    item_name: Any,
    extra_fields: tuple[str, ...] = (),
) -> dict[str, Any]:
    candidate_map = {
        name: item
        for item in candidate
        for name in [clean_text(item_name(item))]
        if name
    }
    fixture_map = {
        name: item
        for item in fixture_post
        for name in [clean_text(item_name(item))]
        if name
    }
    candidate_names = sorted(candidate_map)
    fixture_names = sorted(fixture_map)
    changed: list[dict[str, Any]] = []
    for name in sorted(name for name in candidate_names if name in fixture_map):
        differences = {}
        for field in extra_fields:
            if candidate_map[name].get(field) != fixture_map[name].get(field):
                differences[field] = {
                    "candidate": candidate_map[name].get(field),
                    "fixture_post": fixture_map[name].get(field),
                }
        if differences:
            changed.append({"name": name, "differences": differences})
    return {
        "candidate_names": candidate_names,
        "fixture_names": fixture_names,
        "missing_from_candidate": sorted(name for name in fixture_names if name not in candidate_map),
        "unexpected_in_candidate": sorted(name for name in candidate_names if name not in fixture_map),
        "shared": sorted(name for name in candidate_names if name in fixture_map),
        "changed": changed,
    }


def compare_call_sequences(
    candidate: list[Mapping[str, Any]],
    fixture_post: list[Mapping[str, Any]],
) -> dict[str, Any]:
    candidate_sequence = [clean_text(entry.get("callee", "")) for entry in first_call_sequence(candidate)]
    fixture_sequence = [clean_text(entry.get("callee", "")) for entry in first_call_sequence(fixture_post)]
    candidate_sequence = [value for value in candidate_sequence if value]
    fixture_sequence = [value for value in fixture_sequence if value]
    return {
        "candidate_call_sequence": candidate_sequence,
        "fixture_call_sequence": fixture_sequence,
        "missing_from_candidate": ordered_difference(fixture_sequence, candidate_sequence),
        "unexpected_in_candidate": ordered_difference(candidate_sequence, fixture_sequence),
    }


def first_call_sequence(items: list[Mapping[str, Any]]) -> list[Mapping[str, Any]]:
    if not items:
        return []
    first = items[0]
    calls = first.get("call_sequence", [])
    if not isinstance(calls, list):
        return []
    return [entry for entry in calls if isinstance(entry, Mapping)]


def ordered_difference(left: Iterable[str], right: Iterable[str]) -> list[str]:
    right_set = {value for value in right if value}
    result: list[str] = []
    for value in left:
        if value and value not in right_set and value not in result:
            result.append(value)
    return result


def extract_prediction_type_refs(call: ast.Call, source: str) -> list[str]:
    if not call.args:
        return []
    first = call.args[0]
    if isinstance(first, (ast.List, ast.Tuple)):
        return [render_node(item, source) for item in first.elts]
    return [render_node(first, source)]


def extract_string_argument(call: ast.Call, *, keyword_name: str) -> str:
    keyword_value = extract_keyword_value(call, keyword_name)
    if isinstance(keyword_value, ast.Constant) and isinstance(keyword_value.value, str):
        return keyword_value.value
    if call.args:
        first = call.args[0]
        if isinstance(first, ast.Constant) and isinstance(first.value, str):
            return first.value
    return ""


def extract_int_keyword(call: ast.Call, keyword_name: str) -> int | None:
    value = extract_keyword_value(call, keyword_name)
    if isinstance(value, ast.Constant) and isinstance(value.value, int):
        return value.value
    return None


def extract_bool_keyword(call: ast.Call, keyword_name: str) -> bool | None:
    value = extract_keyword_value(call, keyword_name)
    if isinstance(value, ast.Constant) and isinstance(value.value, bool):
        return value.value
    return None


def extract_keyword_value(call: ast.Call, keyword_name: str) -> ast.AST | None:
    for keyword in call.keywords:
        if keyword.arg == keyword_name:
            return keyword.value
    return None


def literal_sequence_length(node: ast.AST | None) -> int | None:
    if isinstance(node, (ast.List, ast.Tuple)):
        return len(node.elts)
    return None


def provenance_for_function(node: ast.FunctionDef | ast.AsyncFunctionDef, lines: list[str], relative_path: str) -> dict[str, Any]:
    start = node.lineno
    if node.decorator_list:
        start = min(getattr(decorator, "lineno", start) for decorator in node.decorator_list)
    return provenance_for_line_range(start, getattr(node, "end_lineno", node.lineno), lines, relative_path)


def provenance_for_node(node: ast.AST, lines: list[str], relative_path: str, *, max_lines: int = 12) -> dict[str, Any]:
    start = getattr(node, "lineno", 1)
    end = getattr(node, "end_lineno", start)
    return provenance_for_line_range(start, end, lines, relative_path, max_lines=max_lines)


def provenance_for_line_range(
    start: int,
    end: int,
    lines: list[str],
    relative_path: str,
    *,
    max_lines: int = 12,
) -> dict[str, Any]:
    snippet_end = min(end, start + max_lines - 1)
    snippet_lines = lines[start - 1 : snippet_end]
    snippet = "\n".join(snippet_lines).rstrip()
    if snippet_end < end:
        snippet += "\n..."
    return {
        "path": relative_path,
        "line_start": start,
        "line_end": end,
        "snippet": snippet,
    }


def is_main_block(node: ast.AST) -> bool:
    if not isinstance(node, ast.If):
        return False
    test = node.test
    if not isinstance(test, ast.Compare) or len(test.ops) != 1 or len(test.comparators) != 1:
        return False
    if not isinstance(test.ops[0], ast.Eq):
        return False
    left = test.left
    right = test.comparators[0]
    return (
        isinstance(left, ast.Name)
        and left.id == "__name__"
        and isinstance(right, ast.Constant)
        and right.value == "__main__"
    )


def normalized_name(node: ast.AST | None) -> str:
    if isinstance(node, ast.Name):
        return node.id
    if isinstance(node, ast.Attribute):
        base = normalized_name(node.value)
        return f"{base}.{node.attr}" if base else node.attr
    if isinstance(node, ast.Call):
        return normalized_name(node.func)
    return ""


def render_node(node: ast.AST | None, source: str) -> str:
    if node is None:
        return ""
    rendered = ast.get_source_segment(source, node)
    if rendered is not None:
        return rendered.strip()
    return ""


def sort_key(item: Any) -> tuple[str, str, int]:
    if isinstance(item, Mapping):
        path = clean_text(item.get("path", "")) or clean_text(item.get("function", ""))
        name = clean_text(item.get("name", "")) or clean_text(item.get("variable", "")) or clean_text(item.get("function", ""))
        line = int(item.get("line_start", 0) or 0)
        provenance = item.get("provenance", {})
        if isinstance(provenance, Mapping):
            path = clean_text(provenance.get("path", "")) or path
            line = int(provenance.get("line_start", line) or line)
        return (path, name, line)
    return ("", str(item), 0)


def clean_text(value: Any) -> str:
    return str(value or "").strip()

package core

// IssueCode is a stable machine-readable issue identifier.
// New codes can be added as Concierge learns additional failure modes.
type IssueCode string

const (
	IssueCodeUnknown IssueCode = "unknown"

	// Repository and workspace context.
	IssueCodeRepositoryNotGit         IssueCode = "repository_not_git"
	IssueCodeProjectRootInvalid       IssueCode = "project_root_invalid"
	IssueCodeProjectRootAmbiguous     IssueCode = "project_root_ambiguous"
	IssueCodeProjectRootUnselected    IssueCode = "project_root_unselected"
	IssueCodeWorkingTreeDirty         IssueCode = "working_tree_dirty"
	IssueCodeIntegrationBranchMissing IssueCode = "integration_branch_missing"

	// Runtime dependencies.
	IssueCodePythonNotFound           IssueCode = "python_not_found"
	IssueCodePythonVersionUnsupported IssueCode = "python_version_unsupported"
	IssueCodeRequirementsMissing      IssueCode = "requirements_missing"
	IssueCodeRequirementsParseFailed  IssueCode = "requirements_parse_failed"

	// Tensorleap CLI, auth, server, and secrets.
	IssueCodeLeapCLINotFound                 IssueCode = "leap_cli_not_found"
	IssueCodeLeapCLIVersionUnavailable       IssueCode = "leap_cli_version_unavailable"
	IssueCodeLeapCLINotAuthenticated         IssueCode = "leap_cli_not_authenticated"
	IssueCodeLeapServerUnreachable           IssueCode = "leap_server_unreachable"
	IssueCodeLeapServerInfoFailed            IssueCode = "leap_server_info_failed"
	IssueCodeLeapServerDatasetVolumesMissing IssueCode = "leap_server_datasetvolumes_missing"
	IssueCodeLeapSecretMissing               IssueCode = "leap_secret_missing"
	IssueCodeLeapSecretAccessFailed          IssueCode = "leap_secret_access_failed"
	IssueCodeAuthSecretUnavailable           IssueCode = "auth_secret_unavailable"

	// leap.yaml and upload boundary contract.
	IssueCodeLeapYAMLMissing                     IssueCode = "leap_yaml_missing"
	IssueCodeLeapYAMLUnparseable                 IssueCode = "leap_yaml_unparseable"
	IssueCodeLeapYAMLEntryFileMissing            IssueCode = "leap_yaml_entry_file_missing"
	IssueCodeLeapYAMLEntryFileInvalid            IssueCode = "leap_yaml_entry_file_invalid"
	IssueCodeLeapYAMLEntryFileNotFound           IssueCode = "leap_yaml_entry_file_not_found"
	IssueCodeLeapYAMLEntryFileOutsideRepo        IssueCode = "leap_yaml_entry_file_outside_repo"
	IssueCodeLeapYAMLEntryFileExcluded           IssueCode = "leap_yaml_entry_file_excluded"
	IssueCodeLeapYAMLIncludeMissingRequiredFiles IssueCode = "leap_yaml_include_missing_required_files"
	IssueCodeLeapYAMLExcludeBlocksRequiredFiles  IssueCode = "leap_yaml_exclude_blocks_required_files"
	IssueCodeLeapYAMLPythonVersionMissing        IssueCode = "leap_yaml_python_version_missing"
	IssueCodeLeapYAMLPythonVersionInvalid        IssueCode = "leap_yaml_python_version_invalid"

	// Model contract.
	IssueCodeModelFileMissing                 IssueCode = "model_file_missing"
	IssueCodeModelCandidatesAmbiguous         IssueCode = "model_candidates_ambiguous"
	IssueCodeModelFormatUnsupported           IssueCode = "model_format_unsupported"
	IssueCodeModelLoadFailed                  IssueCode = "model_load_failed"
	IssueCodeModelInputBatchDimensionMissing  IssueCode = "model_input_batch_dimension_missing"
	IssueCodeModelOutputBatchDimensionMissing IssueCode = "model_output_batch_dimension_missing"
	IssueCodeModelInputShapeMismatch          IssueCode = "model_input_shape_mismatch"
	IssueCodeModelOutputShapeMismatch         IssueCode = "model_output_shape_mismatch"

	// Integration files and decorators.
	IssueCodeIntegrationScriptMissing               IssueCode = "integration_script_missing"
	IssueCodeIntegrationScriptImportFailed          IssueCode = "integration_script_import_failed"
	IssueCodeIntegrationTestMissing                 IssueCode = "integration_test_missing"
	IssueCodeIntegrationTestDecoratorMissing        IssueCode = "integration_test_decorator_missing"
	IssueCodeLoadModelDecoratorMissing              IssueCode = "load_model_decorator_missing"
	IssueCodeIntegrationTestExecutionFailed         IssueCode = "integration_test_execution_failed"
	IssueCodeIntegrationTestMissingRequiredCalls    IssueCode = "integration_test_missing_required_calls"
	IssueCodeIntegrationTestCallsUnknownInterfaces  IssueCode = "integration_test_calls_unknown_interfaces"
	IssueCodeIntegrationTestManualBatchManipulation IssueCode = "integration_test_manual_batch_manipulation"

	// Preprocess contract.
	IssueCodePreprocessFunctionMissing         IssueCode = "preprocess_function_missing"
	IssueCodePreprocessExecutionFailed         IssueCode = "preprocess_execution_failed"
	IssueCodePreprocessResponseInvalid         IssueCode = "preprocess_response_invalid"
	IssueCodePreprocessTrainSubsetMissing      IssueCode = "preprocess_train_subset_missing"
	IssueCodePreprocessValidationSubsetMissing IssueCode = "preprocess_validation_subset_missing"
	IssueCodePreprocessSubsetEmpty             IssueCode = "preprocess_subset_empty"

	// Input encoder contract.
	IssueCodeInputEncoderMissing                 IssueCode = "input_encoder_missing"
	IssueCodeInputEncoderExecutionFailed         IssueCode = "input_encoder_execution_failed"
	IssueCodeInputEncoderShapeInvalid            IssueCode = "input_encoder_shape_invalid"
	IssueCodeInputEncoderDTypeInvalid            IssueCode = "input_encoder_dtype_invalid"
	IssueCodeInputEncoderNonFiniteValues         IssueCode = "input_encoder_non_finite_values"
	IssueCodeInputEncoderCoverageIncomplete      IssueCode = "input_encoder_coverage_incomplete"
	IssueCodeInputEncoderConstantOutputSuspected IssueCode = "input_encoder_constant_output_suspected"

	// Ground-truth encoder contract.
	IssueCodeGTEncoderMissing                 IssueCode = "gt_encoder_missing"
	IssueCodeGTEncoderExecutionFailed         IssueCode = "gt_encoder_execution_failed"
	IssueCodeGTEncoderShapeInvalid            IssueCode = "gt_encoder_shape_invalid"
	IssueCodeGTEncoderDTypeInvalid            IssueCode = "gt_encoder_dtype_invalid"
	IssueCodeGTEncoderNonFiniteValues         IssueCode = "gt_encoder_non_finite_values"
	IssueCodeGTEncoderCoverageIncomplete      IssueCode = "gt_encoder_coverage_incomplete"
	IssueCodeGTEncoderConstantOutputSuspected IssueCode = "gt_encoder_constant_output_suspected"
	IssueCodeUnlabeledSubsetGTInvocation      IssueCode = "unlabeled_subset_gt_invocation"

	// Upload and runtime validation.
	IssueCodeUploadNotConfirmed               IssueCode = "upload_not_confirmed"
	IssueCodeUploadFailed                     IssueCode = "upload_failed"
	IssueCodeUploadedFilesMissingArtifacts    IssueCode = "uploaded_files_missing_required_artifacts"
	IssueCodeDatasetPathMissing               IssueCode = "dataset_path_missing"
	IssueCodeDatasetPathNotMounted            IssueCode = "dataset_path_not_mounted"
	IssueCodeHarnessPreprocessFailed          IssueCode = "harness_preprocess_failed"
	IssueCodeHarnessEncoderCoverageIncomplete IssueCode = "harness_encoder_coverage_incomplete"
	IssueCodeHarnessValidationFailed          IssueCode = "harness_validation_failed"

	// Anti-stub heuristics.
	IssueCodeSuspiciousConstantInputs      IssueCode = "suspicious_constant_inputs"
	IssueCodeSuspiciousConstantLabels      IssueCode = "suspicious_constant_labels"
	IssueCodeSuspiciousConstantPredictions IssueCode = "suspicious_constant_predictions"
)

// IssueScope tells where an issue applies. Location is optional and may be omitted.
type IssueScope string

const (
	IssueScopeWorkspace          IssueScope = "workspace"
	IssueScopeRepository         IssueScope = "repository"
	IssueScopeEnvironment        IssueScope = "environment"
	IssueScopeCLI                IssueScope = "cli"
	IssueScopeServer             IssueScope = "server"
	IssueScopeSecrets            IssueScope = "secrets"
	IssueScopeLeapYAML           IssueScope = "leap_yaml"
	IssueScopeModel              IssueScope = "model"
	IssueScopeIntegrationScript  IssueScope = "integration_script"
	IssueScopeIntegrationTest    IssueScope = "integration_test"
	IssueScopePreprocess         IssueScope = "preprocess"
	IssueScopeInputEncoder       IssueScope = "input_encoder"
	IssueScopeGroundTruthEncoder IssueScope = "ground_truth_encoder"
	IssueScopeUpload             IssueScope = "upload"
	IssueScopeValidation         IssueScope = "validation"
	IssueScopeDataset            IssueScope = "dataset"
)

var knownIssueCodes = []IssueCode{
	IssueCodeUnknown,
	IssueCodeRepositoryNotGit,
	IssueCodeProjectRootInvalid,
	IssueCodeProjectRootAmbiguous,
	IssueCodeProjectRootUnselected,
	IssueCodeWorkingTreeDirty,
	IssueCodeIntegrationBranchMissing,
	IssueCodePythonNotFound,
	IssueCodePythonVersionUnsupported,
	IssueCodeRequirementsMissing,
	IssueCodeRequirementsParseFailed,
	IssueCodeLeapCLINotFound,
	IssueCodeLeapCLIVersionUnavailable,
	IssueCodeLeapCLINotAuthenticated,
	IssueCodeLeapServerUnreachable,
	IssueCodeLeapServerInfoFailed,
	IssueCodeLeapServerDatasetVolumesMissing,
	IssueCodeLeapSecretMissing,
	IssueCodeLeapSecretAccessFailed,
	IssueCodeAuthSecretUnavailable,
	IssueCodeLeapYAMLMissing,
	IssueCodeLeapYAMLUnparseable,
	IssueCodeLeapYAMLEntryFileMissing,
	IssueCodeLeapYAMLEntryFileInvalid,
	IssueCodeLeapYAMLEntryFileNotFound,
	IssueCodeLeapYAMLEntryFileOutsideRepo,
	IssueCodeLeapYAMLEntryFileExcluded,
	IssueCodeLeapYAMLIncludeMissingRequiredFiles,
	IssueCodeLeapYAMLExcludeBlocksRequiredFiles,
	IssueCodeLeapYAMLPythonVersionMissing,
	IssueCodeLeapYAMLPythonVersionInvalid,
	IssueCodeModelFileMissing,
	IssueCodeModelCandidatesAmbiguous,
	IssueCodeModelFormatUnsupported,
	IssueCodeModelLoadFailed,
	IssueCodeModelInputBatchDimensionMissing,
	IssueCodeModelOutputBatchDimensionMissing,
	IssueCodeModelInputShapeMismatch,
	IssueCodeModelOutputShapeMismatch,
	IssueCodeIntegrationScriptMissing,
	IssueCodeIntegrationScriptImportFailed,
	IssueCodeIntegrationTestMissing,
	IssueCodeIntegrationTestDecoratorMissing,
	IssueCodeLoadModelDecoratorMissing,
	IssueCodeIntegrationTestExecutionFailed,
	IssueCodeIntegrationTestMissingRequiredCalls,
	IssueCodeIntegrationTestCallsUnknownInterfaces,
	IssueCodeIntegrationTestManualBatchManipulation,
	IssueCodePreprocessFunctionMissing,
	IssueCodePreprocessExecutionFailed,
	IssueCodePreprocessResponseInvalid,
	IssueCodePreprocessTrainSubsetMissing,
	IssueCodePreprocessValidationSubsetMissing,
	IssueCodePreprocessSubsetEmpty,
	IssueCodeInputEncoderMissing,
	IssueCodeInputEncoderExecutionFailed,
	IssueCodeInputEncoderShapeInvalid,
	IssueCodeInputEncoderDTypeInvalid,
	IssueCodeInputEncoderNonFiniteValues,
	IssueCodeInputEncoderCoverageIncomplete,
	IssueCodeInputEncoderConstantOutputSuspected,
	IssueCodeGTEncoderMissing,
	IssueCodeGTEncoderExecutionFailed,
	IssueCodeGTEncoderShapeInvalid,
	IssueCodeGTEncoderDTypeInvalid,
	IssueCodeGTEncoderNonFiniteValues,
	IssueCodeGTEncoderCoverageIncomplete,
	IssueCodeGTEncoderConstantOutputSuspected,
	IssueCodeUnlabeledSubsetGTInvocation,
	IssueCodeUploadNotConfirmed,
	IssueCodeUploadFailed,
	IssueCodeUploadedFilesMissingArtifacts,
	IssueCodeDatasetPathMissing,
	IssueCodeDatasetPathNotMounted,
	IssueCodeHarnessPreprocessFailed,
	IssueCodeHarnessEncoderCoverageIncomplete,
	IssueCodeHarnessValidationFailed,
	IssueCodeSuspiciousConstantInputs,
	IssueCodeSuspiciousConstantLabels,
	IssueCodeSuspiciousConstantPredictions,
}

var knownIssueCodeSet = func() map[IssueCode]struct{} {
	set := make(map[IssueCode]struct{}, len(knownIssueCodes))
	for _, code := range knownIssueCodes {
		set[code] = struct{}{}
	}
	return set
}()

// KnownIssueCodes returns a copy of the seeded issue code catalog.
func KnownIssueCodes() []IssueCode {
	codes := make([]IssueCode, len(knownIssueCodes))
	copy(codes, knownIssueCodes)
	return codes
}

// IsKnownIssueCode reports whether code is in Concierge's seeded catalog.
func IsKnownIssueCode(code IssueCode) bool {
	_, ok := knownIssueCodeSet[code]
	return ok
}

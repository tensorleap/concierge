package core

var preferredEnsureStepByIssueCode = map[IssueCode]EnsureStepID{
	IssueCodeUnknown: EnsureStepInvestigate,

	// Repository and workspace context.
	IssueCodeRepositoryNotGit:         EnsureStepRepositoryContext,
	IssueCodeProjectRootInvalid:       EnsureStepRepositoryContext,
	IssueCodeProjectRootAmbiguous:     EnsureStepRepositoryContext,
	IssueCodeProjectRootUnselected:    EnsureStepRepositoryContext,
	IssueCodeWorkingTreeDirty:         EnsureStepRepositoryContext,
	IssueCodeIntegrationBranchMissing: EnsureStepRepositoryContext,

	// Runtime dependencies.
	IssueCodePoetryNotFound:                EnsureStepPythonRuntime,
	IssueCodeRuntimeProjectUnsupported:     EnsureStepPythonRuntime,
	IssueCodePoetryEnvironmentUnresolved:   EnsureStepPythonRuntime,
	IssueCodeRuntimeProfileDrifted:         EnsureStepPythonRuntime,
	IssueCodePoetryCheckFailed:             EnsureStepPythonRuntime,
	IssueCodeCodeLoaderMissing:             EnsureStepPythonRuntime,
	IssueCodeCodeLoaderLegacy:              EnsureStepPythonRuntime,
	IssueCodeNativeSystemDependencyMissing: EnsureStepPythonRuntime,
	IssueCodePythonVersionUnsupported:      EnsureStepPythonRuntime,
	IssueCodeRequirementsMissing:           EnsureStepPythonRuntime,
	IssueCodeRequirementsParseFailed:       EnsureStepPythonRuntime,

	// Tensorleap CLI, auth, server, and secrets.
	IssueCodeLeapCLINotFound:                 EnsureStepLeapCLIAuth,
	IssueCodeLeapCLIVersionUnavailable:       EnsureStepLeapCLIAuth,
	IssueCodeLeapCLINotAuthenticated:         EnsureStepLeapCLIAuth,
	IssueCodeLeapServerUnreachable:           EnsureStepServerConnectivity,
	IssueCodeLeapServerInfoFailed:            EnsureStepServerConnectivity,
	IssueCodeLeapServerDatasetVolumesMissing: EnsureStepServerConnectivity,
	IssueCodeLeapSecretMissing:               EnsureStepSecretsContext,
	IssueCodeLeapSecretAccessFailed:          EnsureStepSecretsContext,
	IssueCodeAuthSecretUnavailable:           EnsureStepSecretsContext,

	// leap.yaml and upload boundary contract.
	IssueCodeLeapYAMLMissing:                     EnsureStepLeapYAML,
	IssueCodeLeapYAMLUnparseable:                 EnsureStepLeapYAML,
	IssueCodeLeapYAMLEntryFileMissing:            EnsureStepLeapYAML,
	IssueCodeLeapYAMLEntryFileInvalid:            EnsureStepLeapYAML,
	IssueCodeLeapYAMLEntryFileNotFound:           EnsureStepLeapYAML,
	IssueCodeLeapYAMLEntryFileOutsideRepo:        EnsureStepLeapYAML,
	IssueCodeLeapYAMLEntryFileExcluded:           EnsureStepLeapYAML,
	IssueCodeLeapYAMLIncludeMissingRequiredFiles: EnsureStepLeapYAML,
	IssueCodeLeapYAMLExcludeBlocksRequiredFiles:  EnsureStepLeapYAML,
	IssueCodeLeapYAMLPythonVersionMissing:        EnsureStepLeapYAML,
	IssueCodeLeapYAMLPythonVersionInvalid:        EnsureStepLeapYAML,

	// Model contract.
	IssueCodeModelAcquisitionRequired:          EnsureStepModelAcquisition,
	IssueCodeModelAcquisitionUnresolved:        EnsureStepModelAcquisition,
	IssueCodeModelMaterializationFailed:        EnsureStepModelAcquisition,
	IssueCodeModelMaterializationOutputMissing: EnsureStepModelAcquisition,
	IssueCodeModelFileMissing:                  EnsureStepModelAcquisition,
	IssueCodeModelCandidatesAmbiguous:          EnsureStepModelAcquisition,
	IssueCodeModelFormatUnsupported:            EnsureStepModelAcquisition,
	IssueCodeModelLoadFailed:                   EnsureStepModelContract,
	IssueCodeModelInputBatchDimensionMissing:   EnsureStepModelContract,
	IssueCodeModelOutputBatchDimensionMissing:  EnsureStepModelContract,
	IssueCodeModelInputShapeMismatch:           EnsureStepModelContract,
	IssueCodeModelOutputShapeMismatch:          EnsureStepModelContract,

	// Integration files and decorators.
	IssueCodeIntegrationScriptMissing:               EnsureStepIntegrationScript,
	IssueCodeIntegrationScriptNonCanonical:          EnsureStepIntegrationScript,
	IssueCodeIntegrationScriptImportFailed:          EnsureStepIntegrationScript,
	IssueCodeIntegrationTestMissing:                 EnsureStepIntegrationTestContract,
	IssueCodeIntegrationTestDecoratorMissing:        EnsureStepIntegrationTestContract,
	IssueCodeLoadModelDecoratorMissing:              EnsureStepModelContract,
	IssueCodeIntegrationTestExecutionFailed:         EnsureStepIntegrationTestContract,
	IssueCodeIntegrationTestMissingRequiredCalls:    EnsureStepIntegrationTestContract,
	IssueCodeIntegrationTestCallsUnknownInterfaces:  EnsureStepIntegrationTestContract,
	IssueCodeIntegrationTestDirectDatasetAccess:     EnsureStepIntegrationTestContract,
	IssueCodeIntegrationTestIllegalBodyLogic:        EnsureStepIntegrationTestContract,
	IssueCodeIntegrationTestManualBatchManipulation: EnsureStepIntegrationTestContract,
	IssueCodeIntegrationTestMainBlockMissing:        EnsureStepIntegrationTestContract,

	// Preprocess contract.
	IssueCodePreprocessFunctionMissing:         EnsureStepPreprocessContract,
	IssueCodePreprocessExecutionFailed:         EnsureStepPreprocessContract,
	IssueCodePreprocessResponseInvalid:         EnsureStepPreprocessContract,
	IssueCodePreprocessTrainSubsetMissing:      EnsureStepPreprocessContract,
	IssueCodePreprocessValidationSubsetMissing: EnsureStepPreprocessContract,
	IssueCodePreprocessSubsetEmpty:             EnsureStepPreprocessContract,

	// Input encoder contract.
	IssueCodeInputEncoderMissing:                 EnsureStepInputEncoders,
	IssueCodeInputEncoderExecutionFailed:         EnsureStepInputEncoders,
	IssueCodeInputEncoderShapeInvalid:            EnsureStepInputEncoders,
	IssueCodeInputEncoderDTypeInvalid:            EnsureStepInputEncoders,
	IssueCodeInputEncoderNonFiniteValues:         EnsureStepInputEncoders,
	IssueCodeInputEncoderCoverageIncomplete:      EnsureStepInputEncoders,
	IssueCodeInputEncoderConstantOutputSuspected: EnsureStepInputEncoders,

	// Ground-truth encoder contract.
	IssueCodeGTEncoderMissing:                 EnsureStepGroundTruthEncoders,
	IssueCodeGTEncoderExecutionFailed:         EnsureStepGroundTruthEncoders,
	IssueCodeGTEncoderShapeInvalid:            EnsureStepGroundTruthEncoders,
	IssueCodeGTEncoderDTypeInvalid:            EnsureStepGroundTruthEncoders,
	IssueCodeGTEncoderNonFiniteValues:         EnsureStepGroundTruthEncoders,
	IssueCodeGTEncoderCoverageIncomplete:      EnsureStepGroundTruthEncoders,
	IssueCodeGTEncoderConstantOutputSuspected: EnsureStepGroundTruthEncoders,
	IssueCodeUnlabeledSubsetGTInvocation:      EnsureStepGroundTruthEncoders,

	// Upload and runtime validation.
	IssueCodeUploadNotConfirmed:               EnsureStepUploadReadiness,
	IssueCodeUploadFailed:                     EnsureStepUploadPush,
	IssueCodeUploadedFilesMissingArtifacts:    EnsureStepUploadReadiness,
	IssueCodeDatasetPathMissing:               EnsureStepUploadReadiness,
	IssueCodeDatasetPathNotMounted:            EnsureStepUploadReadiness,
	IssueCodeHarnessPreprocessFailed:          EnsureStepHarnessValidation,
	IssueCodeHarnessEncoderCoverageIncomplete: EnsureStepHarnessValidation,
	IssueCodeHarnessValidationFailed:          EnsureStepHarnessValidation,

	// Anti-stub heuristics.
	IssueCodeSuspiciousConstantInputs:      EnsureStepHarnessValidation,
	IssueCodeSuspiciousConstantLabels:      EnsureStepHarnessValidation,
	IssueCodeSuspiciousConstantPredictions: EnsureStepHarnessValidation,
}

// PreferredEnsureStepForIssueCode returns a preferred ensure-step for known issue codes.
func PreferredEnsureStepForIssueCode(code IssueCode) (EnsureStep, bool) {
	stepID, ok := preferredEnsureStepByIssueCode[code]
	if !ok {
		return EnsureStep{}, false
	}

	step, ok := EnsureStepByID(stepID)
	if !ok {
		return EnsureStep{}, false
	}

	return step, true
}

// PreferredEnsureStepForIssue returns the preferred step and falls back to investigate for unknown codes.
func PreferredEnsureStepForIssue(issue Issue) EnsureStep {
	if step, ok := PreferredEnsureStepForIssueCode(issue.Code); ok {
		return step
	}

	fallback, ok := EnsureStepByID(EnsureStepInvestigate)
	if !ok {
		return EnsureStep{}
	}
	return fallback
}

// PreferredEnsureStepsForIssues returns unique preferred steps in canonical planner priority order.
func PreferredEnsureStepsForIssues(issues []Issue) []EnsureStep {
	if len(issues) == 0 {
		return nil
	}

	candidateSet := make(map[EnsureStepID]struct{}, len(issues))
	for _, issue := range issues {
		step := PreferredEnsureStepForIssue(issue)
		if step.ID == "" {
			continue
		}
		candidateSet[step.ID] = struct{}{}
	}

	steps := make([]EnsureStep, 0, len(candidateSet))
	for _, id := range ensureStepPriority {
		if _, ok := candidateSet[id]; !ok {
			continue
		}
		step, ok := ensureStepCatalog[id]
		if !ok {
			continue
		}
		steps = append(steps, step)
	}

	return steps
}

// SelectPrimaryEnsureStep picks the highest-priority preferred step for a set of issues.
func SelectPrimaryEnsureStep(issues []Issue) (EnsureStep, bool) {
	steps := PreferredEnsureStepsForIssues(issues)
	if len(steps) == 0 {
		return EnsureStep{}, false
	}
	return steps[0], true
}

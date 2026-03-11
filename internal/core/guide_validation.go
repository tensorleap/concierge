package core

import (
	"encoding/json"
	"strings"
)

const (
	GuideEvidenceSummary      = "guide.summary.json"
	GuideEvidenceLocalRaw     = "guide.local.summary.json"
	GuideEvidenceParserRaw    = "guide.parser.result.json"
	GuideEvidenceLocalCmd     = "guide.local.command"
	GuideEvidenceLocalStdout  = "guide.local.stdout"
	GuideEvidenceLocalStderr  = "guide.local.stderr"
	GuideEvidenceParserCmd    = "guide.parser.command"
	GuideEvidenceParserStdout = "guide.parser.stdout"
	GuideEvidenceParserStderr = "guide.parser.stderr"
)

// GuideValidationSummary captures guide-native validation milestones for one iteration.
type GuideValidationSummary struct {
	Skipped        bool                  `json:"skipped,omitempty"`
	SkipReason     string                `json:"skipReason,omitempty"`
	Local          GuideLocalRunSummary  `json:"local,omitempty"`
	Parser         GuideParserRunSummary `json:"parser,omitempty"`
	Recommendation GuideRecommendation   `json:"recommendation,omitempty"`
}

// GuideLocalRunSummary captures author-facing local validator signals from running leap_integration.py directly.
type GuideLocalRunSummary struct {
	Attempted            bool             `json:"attempted,omitempty"`
	Successful           bool             `json:"successful,omitempty"`
	MappingFailure       bool             `json:"mappingFailure,omitempty"`
	CrashFunction        string           `json:"crashFunction,omitempty"`
	RecommendedInterface string           `json:"recommendedInterface,omitempty"`
	MandatoryReady       bool             `json:"mandatoryReady,omitempty"`
	AllPartsReady        bool             `json:"allPartsReady,omitempty"`
	DefaultWarnings      []string         `json:"defaultWarnings,omitempty"`
	StatusRows           []GuideStatusRow `json:"statusRows,omitempty"`
}

// GuideStatusRow captures one status-table row from code-loader's local validator.
type GuideStatusRow struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

// GuideParserRunSummary captures the machine-readable result of LeapLoader.check_dataset().
type GuideParserRunSummary struct {
	Attempted         bool                      `json:"attempted,omitempty"`
	Available         bool                      `json:"available,omitempty"`
	UnavailableReason string                    `json:"unavailableReason,omitempty"`
	IsValid           bool                      `json:"isValid,omitempty"`
	IsValidForModel   bool                      `json:"isValidForModel,omitempty"`
	GeneralError      string                    `json:"generalError,omitempty"`
	PrintLog          string                    `json:"printLog,omitempty"`
	Payloads          []GuidePayloadSummary     `json:"payloads,omitempty"`
	Setup             *GuideDatasetSetupSummary `json:"setup,omitempty"`
	ModelSetup        *GuideModelSetupSummary   `json:"modelSetup,omitempty"`
	EngineFile        *GuideEngineFileSummary   `json:"engineFileContract,omitempty"`
}

// GuidePayloadSummary captures one normalized payload entry from DatasetIntegParseResult.
type GuidePayloadSummary struct {
	Name        string            `json:"name"`
	HandlerType string            `json:"handlerType,omitempty"`
	Passed      bool              `json:"passed"`
	Display     map[string]string `json:"display,omitempty"`
	Shape       []int             `json:"shape,omitempty"`
	Error       string            `json:"error,omitempty"`
}

// GuideDatasetSetupSummary captures the stable setup fields needed for reporting/debugging.
type GuideDatasetSetupSummary struct {
	Preprocess      *GuidePreprocessSummary      `json:"preprocess,omitempty"`
	Inputs          []GuideNamedShapeSummary     `json:"inputs,omitempty"`
	Metadata        []GuideNamedTypeSummary      `json:"metadata,omitempty"`
	Outputs         []GuideNamedShapeSummary     `json:"outputs,omitempty"`
	Visualizers     []GuideNamedArgsSummary      `json:"visualizers,omitempty"`
	PredictionTypes []GuidePredictionTypeSummary `json:"predictionTypes,omitempty"`
	CustomLosses    []GuideNamedArgsSummary      `json:"customLosses,omitempty"`
	Metrics         []GuideNamedArgsSummary      `json:"metrics,omitempty"`
}

// GuidePreprocessSummary captures subset lengths from the parser result.
type GuidePreprocessSummary struct {
	TrainingLength   int `json:"trainingLength,omitempty"`
	ValidationLength int `json:"validationLength,omitempty"`
	TestLength       int `json:"testLength,omitempty"`
	UnlabeledLength  int `json:"unlabeledLength,omitempty"`
	AdditionalLength int `json:"additionalLength,omitempty"`
}

// GuideNamedShapeSummary captures handler or tensor name + shape metadata.
type GuideNamedShapeSummary struct {
	Name       string `json:"name"`
	ChannelDim int    `json:"channelDim,omitempty"`
	Shape      []int  `json:"shape,omitempty"`
}

// GuideNamedTypeSummary captures metadata names and stable types.
type GuideNamedTypeSummary struct {
	Name string `json:"name"`
	Type string `json:"type,omitempty"`
}

// GuideNamedArgsSummary captures named handlers with their argument names.
type GuideNamedArgsSummary struct {
	Name     string   `json:"name"`
	ArgNames []string `json:"argNames,omitempty"`
	Type     string   `json:"type,omitempty"`
}

// GuidePredictionTypeSummary captures normalized prediction type information.
type GuidePredictionTypeSummary struct {
	Name       string   `json:"name"`
	Labels     []string `json:"labels,omitempty"`
	ChannelDim int      `json:"channelDim,omitempty"`
}

// GuideModelSetupSummary captures normalized custom-layer information.
type GuideModelSetupSummary struct {
	CustomLayers []GuideCustomLayerSummary `json:"customLayers,omitempty"`
}

// GuideCustomLayerSummary captures one custom-layer declaration from model setup.
type GuideCustomLayerSummary struct {
	Name                 string   `json:"name"`
	InitArgNames         []string `json:"initArgNames,omitempty"`
	CallArgNames         []string `json:"callArgNames,omitempty"`
	UseCustomLatentSpace bool     `json:"useCustomLatentSpace,omitempty"`
}

// GuideEngineFileSummary captures a reduced engine-file contract summary.
type GuideEngineFileSummary struct {
	NodeConnectionCount int      `json:"nodeConnectionCount,omitempty"`
	FeatureFlags        []string `json:"featureFlags,omitempty"`
	DomainGapMetadata   []string `json:"domainGapMetadata,omitempty"`
}

// GuideRecommendation captures the next useful guide-native milestone to pursue.
type GuideRecommendation struct {
	Stage   string `json:"stage,omitempty"`
	Message string `json:"message,omitempty"`
}

// ParseGuideValidationSummary extracts the guide summary from iteration evidence when present.
func ParseGuideValidationSummary(evidence []EvidenceItem) (GuideValidationSummary, bool) {
	for _, item := range evidence {
		if item.Name != GuideEvidenceSummary {
			continue
		}
		if strings.TrimSpace(item.Value) == "" {
			return GuideValidationSummary{}, false
		}
		var summary GuideValidationSummary
		if err := json.Unmarshal([]byte(item.Value), &summary); err != nil {
			return GuideValidationSummary{}, false
		}
		return summary, true
	}
	return GuideValidationSummary{}, false
}

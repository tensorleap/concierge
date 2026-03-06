package inspect

const (
	frameworkLeadSchemaVersion = "1.0.0"
	frameworkLeadMethodVersion = "framework-leads-v1"
)

type frameworkLeadSignalDefinition struct {
	ID          string
	Description string
	Tier        string
	Framework   string
	Weight      float64
	Pattern     string
}

var frameworkLeadSignalDefinitions = []frameworkLeadSignalDefinition{
	// PyTorch-specific.
	{
		ID:          "torch_import",
		Description: "Imports torch or modules under torch.*",
		Tier:        "primary",
		Framework:   "pytorch",
		Weight:      5.0,
		Pattern:     `\bimport\s+torch\b|\bfrom\s+torch\b`,
	},
	{
		ID:          "dataloader_import",
		Description: "Imports DataLoader from torch.utils.data",
		Tier:        "primary",
		Framework:   "pytorch",
		Weight:      7.0,
		Pattern:     `\bfrom\s+torch\.utils\.data\s+import\s+.*\bDataLoader\b|\bimport\s+torch\.utils\.data\b`,
	},
	{
		ID:          "dataloader_call",
		Description: "DataLoader(...) call site",
		Tier:        "primary",
		Framework:   "pytorch",
		Weight:      10.0,
		Pattern:     `\bDataLoader\s*\(`,
	},
	{
		ID:          "dataset_subclass",
		Description: "class X(...Dataset...) definition",
		Tier:        "primary",
		Framework:   "pytorch",
		Weight:      8.0,
		Pattern:     `^\s*class\s+\w+\s*\([^)]*Dataset[^)]*\)\s*:`,
	},
	{
		ID:          "dataset_import",
		Description: "Dataset import from torch.utils.data",
		Tier:        "primary",
		Framework:   "pytorch",
		Weight:      5.0,
		Pattern:     `\bfrom\s+torch\.utils\.data\s+import\s+.*\bDataset\b`,
	},
	{
		ID:          "torch_forward_def",
		Description: "forward(...) method definition",
		Tier:        "secondary",
		Framework:   "pytorch",
		Weight:      4.0,
		Pattern:     `^\s*def\s+forward\s*\(`,
	},
	{
		ID:          "torch_load",
		Description: "torch.load(...) call",
		Tier:        "secondary",
		Framework:   "pytorch",
		Weight:      3.0,
		Pattern:     `\btorch\.load\s*\(`,
	},
	// TensorFlow / Keras-specific.
	{
		ID:          "tensorflow_import",
		Description: "Imports tensorflow",
		Tier:        "primary",
		Framework:   "tensorflow",
		Weight:      6.0,
		Pattern:     `\bimport\s+tensorflow\b|\bimport\s+tensorflow\s+as\s+tf\b|\bfrom\s+tensorflow\b`,
	},
	{
		ID:          "keras_import",
		Description: "Imports keras APIs",
		Tier:        "primary",
		Framework:   "tensorflow",
		Weight:      5.0,
		Pattern:     `\bfrom\s+tensorflow\.keras\b|\bfrom\s+keras\b|\bimport\s+keras\b`,
	},
	{
		ID:          "tf_data_dataset",
		Description: "tf.data.Dataset usage",
		Tier:        "primary",
		Framework:   "tensorflow",
		Weight:      9.0,
		Pattern:     `\btf\.data\.Dataset\b`,
	},
	{
		ID:          "tf_data_constructors",
		Description: "tf.data constructor usage",
		Tier:        "primary",
		Framework:   "tensorflow",
		Weight:      8.0,
		Pattern:     `\b(from_tensor_slices|from_generator|list_files)\s*\(`,
	},
	{
		ID:          "tf_record_or_text_dataset",
		Description: "TFRecord/TextLine/Csv dataset reader usage",
		Tier:        "primary",
		Framework:   "tensorflow",
		Weight:      8.0,
		Pattern:     `\b(TFRecordDataset|TextLineDataset|CsvDataset)\s*\(`,
	},
	{
		ID:          "tf_data_pipeline_ops",
		Description: "tf.data pipeline ops (map/batch/shuffle/prefetch/etc.)",
		Tier:        "secondary",
		Framework:   "tensorflow",
		Weight:      6.0,
		Pattern:     `\.(map|batch|padded_batch|shuffle|repeat|prefetch|cache|interleave)\s*\(`,
	},
	{
		ID:          "keras_fit",
		Description: "Keras fit(...) call",
		Tier:        "primary",
		Framework:   "tensorflow",
		Weight:      8.0,
		Pattern:     `\.fit\s*\(`,
	},
	{
		ID:          "keras_evaluate_or_predict",
		Description: "Keras evaluate/predict(...) call",
		Tier:        "secondary",
		Framework:   "tensorflow",
		Weight:      6.0,
		Pattern:     `\.(evaluate|predict)\s*\(`,
	},
	{
		ID:          "keras_dataset_utils",
		Description: "Keras dataset loader utility usage",
		Tier:        "primary",
		Framework:   "tensorflow",
		Weight:      7.0,
		Pattern:     `\b(image_dataset_from_directory|text_dataset_from_directory|audio_dataset_from_directory|timeseries_dataset_from_array)\s*\(`,
	},
	{
		ID:          "keras_sequence_or_pydataset",
		Description: "Keras Sequence/PyDataset usage",
		Tier:        "secondary",
		Framework:   "tensorflow",
		Weight:      5.0,
		Pattern:     `\b(Sequence|PyDataset)\b`,
	},
	{
		ID:          "tfds_load",
		Description: "tensorflow_datasets usage",
		Tier:        "secondary",
		Framework:   "tensorflow",
		Weight:      6.0,
		Pattern:     `\btfds\.load\s*\(|\btensorflow_datasets\b`,
	},
	// Framework-agnostic.
	{
		ID:          "train_fn",
		Description: "Training function definition",
		Tier:        "primary",
		Framework:   "agnostic",
		Weight:      5.0,
		Pattern:     `^\s*def\s+(train|fit|training_step)\b`,
	},
	{
		ID:          "validate_fn",
		Description: "Validation/evaluation function definition",
		Tier:        "primary",
		Framework:   "agnostic",
		Weight:      5.0,
		Pattern:     `^\s*def\s+(validate|validation|val|evaluate|eval|test)\b`,
	},
	{
		ID:          "main_entry",
		Description: "Python __main__ entry point",
		Tier:        "secondary",
		Framework:   "agnostic",
		Weight:      3.0,
		Pattern:     `__name__\s*==\s*['"]__main__['"]`,
	},
	{
		ID:          "batch_unpack_loop",
		Description: "for-loop tuple unpacking pattern",
		Tier:        "secondary",
		Framework:   "agnostic",
		Weight:      4.0,
		Pattern:     `^\s*for\s+[^:\n]*,\s*[^:\n]*\s+in\s+.*:`,
	},
	{
		ID:          "loss_call",
		Description: "loss(...) or criterion(...) usage",
		Tier:        "secondary",
		Framework:   "agnostic",
		Weight:      3.0,
		Pattern:     `\b(loss|criterion)\s*\(`,
	},
	{
		ID:          "model_call",
		Description: "model(...) invocation",
		Tier:        "secondary",
		Framework:   "agnostic",
		Weight:      3.0,
		Pattern:     `\bmodel\s*\(`,
	},
}

var frameworkArtifactSuffixWeights = map[string]map[string]float64{
	"tensorflow": {
		".h5":     14.0,
		".keras":  14.0,
		".tflite": 12.0,
		".pb":     10.0,
	},
	"pytorch": {
		".pt":  14.0,
		".pth": 14.0,
	},
}

var frameworkDependencyFiles = []string{
	"requirements.txt",
	"pyproject.toml",
	"poetry.lock",
	"Pipfile",
	"environment.yml",
	"setup.py",
}

var frameworkDependencyPatterns = map[string][]string{
	"tensorflow": {
		`\btensorflow\b`,
		`\bkeras\b`,
		`\btensorflow-datasets\b`,
		`\btfds\b`,
	},
	"pytorch": {
		`\btorch\b`,
		`\btorchvision\b`,
		`\btorchaudio\b`,
		`\bpytorch-lightning\b`,
		`\blightning\b`,
	},
}

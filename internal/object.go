package internal

type Config struct {
	TargetURLWithHostAndScheme string
	Operator                   string
	NamespacesToInclude        SliceFlag
	NamespacesToExclude        SliceFlag
	PodLabelsToInclude         SliceFlag
	Keywords                   SliceFlag
	BatchSize                  int
	ConfigurationId            string
	OgranisationId             string
	SubscriptionId             string
	EncryptionKey              string
	AuthToken                  string
}

type entry struct {
	Namespace string `json:"namespace"`
	Pod       string `json:"pod"`
	Container string `json:"container"`
	Image     string `json:"image"`
	ImageId   string `json:"imageId"`
	Logs      string `json:"logs"`
	Host      string `json:"host"`
}

package object

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
	Path                       string
}

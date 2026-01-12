package internal

type entry struct {
	Namespace string `json:"namespace"`
	Pod       string `json:"pod"`
	Container string `json:"container"`
	Image     string `json:"image"`
	ImageId   string `json:"imageId"`
	Logs      string `json:"logs"`
	Host      string `json:"host"`
}

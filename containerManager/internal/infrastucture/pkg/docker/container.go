package docker

type CreateOptions struct {
	Name          string
	HostPort      string
	HostIP        string
	ContainerPort string
	Volumes       []string
	Env           []string
}

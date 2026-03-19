package docker

type CreateOptions struct {
	Name                   string
	HostPort               string
	HostIP                 string
	ContainerPort          string
	SecondaryHostPort      string
	SecondaryContainerPort string
	Volumes                []string
	Env                    []string
}

package docker

import (
	"fmt"

	"github.com/docker/go-connections/nat"
)

func volumes(volumes []string) map[string]struct{} {
	volumesMap := make(map[string]struct{})

	for _, volume := range volumes {
		volumesMap[volume] = struct{}{}
	}

	return volumesMap
}

func volumeBinds(volumes []string) []string {
	binds := make([]string, len(volumes))

	for i, volume := range volumes {
		binds[i] = fmt.Sprintf("%s:%s", volume, volume)
	}

	return binds
}

func ports(ports []string) nat.PortMap {
	pMap := make(nat.PortMap)

	for _, port := range ports {
		pMap[nat.Port(port)] = []nat.PortBinding{}
	}

	return pMap
}

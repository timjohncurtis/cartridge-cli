package repair

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/tarantool/cartridge-cli/cli/common"
	"gopkg.in/yaml.v2"
)

const (
	configDirName        = "config"
	configFileName       = "config.yml"
	topologyConfFilename = "topology.yml"

	keyTopology = "topology"

	keyInstances            = "servers"
	keyReplicasets          = "replicasets"
	keyInstanceAdvertiseURI = "uri"
	keyInstanceDisabled     = "disabled"

	keyInstanceReplicasetUUID = "replicaset_uuid"
	keyReplicasetLeaders      = "master"
	keyReplicasetAlias        = "alias"
	keyReplicasetRoles        = "roles"

	expelledState          = "expelled"
	unnamedReplicasetAlias = "unnamed"
)

type RawConfType map[interface{}]interface{}

type InstanceConfType struct {
	AdvertiseURI   string
	ReplicasetUUID string

	IsExpelled bool
	IsDisabled bool

	Raw RawConfType
}

type ReplicasetConfType struct {
	Alias     string
	Instances []string
	Leaders   []string
	Roles     []string

	Raw RawConfType

	LeadersIsString bool
}

type TopologyConfType struct {
	raw         RawConfType
	topologyRaw RawConfType

	Instances    map[string]*InstanceConfType
	instancesRaw RawConfType

	Replicasets    map[string]*ReplicasetConfType
	replicasetsRaw RawConfType
}

type TopologyConfFileInfo struct {
	Path      string
	IsOneFile bool
}

// TOPOLOGY

func getTopologyConfPath(workDir string) (string, error) {
	var topologyConfPath string

	// check config directory <work-dir>/config/
	confDirPath := filepath.Join(workDir, configDirName)
	if _, err := os.Stat(confDirPath); err == nil {
		// find <work-dir>/config/topology.yml
		topologyConfPath = filepath.Join(confDirPath, topologyConfFilename)

		if _, err := os.Stat(topologyConfPath); err == nil {
			return topologyConfPath, nil
		} else if !os.IsNotExist(err) {
			return "", fmt.Errorf("Failed to use topology config file: %s", err)
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("Failed to use clusterwide config directory: %s", err)
	}

	// try old format:  <work-dir>/config.yml
	topologyConfPath = filepath.Join(workDir, configFileName)
	if _, err := os.Stat(topologyConfPath); err == nil {
		return topologyConfPath, nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("Failed to use clusterwide config file: %s", err)
	}

	return "", nil

}

func getTopologyConf(topologyConfPath string) (*TopologyConfType, error) {
	var err error
	var topologyConf TopologyConfType

	if _, err := os.Stat(topologyConfPath); err != nil {
		return nil, fmt.Errorf("Failed to use topology config path: %s", err)
	}

	confContent, err := common.GetFileContentBytes(topologyConfPath)
	if err != nil {
		return nil, fmt.Errorf("Failed to read config: %s", err)
	}

	if err := yaml.Unmarshal(confContent, &topologyConf.raw); err != nil {
		return nil, fmt.Errorf("Failed to parse config: %s", err)
	}

	if err := topologyConf.setTopologyConfRaw(); err != nil {
		return nil, fmt.Errorf("Failed to parse topology config: %s", err)
	}

	if err := topologyConf.setInstancesConf(); err != nil {
		return nil, fmt.Errorf("Failed to parse instances config: %s", err)
	}

	if err := topologyConf.setReplicasetsConf(); err != nil {
		return nil, fmt.Errorf("Failed to get replicasets config: %s", err)
	}

	return &topologyConf, nil
}

func (topologyConf *TopologyConfType) setTopologyConfRaw() error {
	topologyConfRaw, found := topologyConf.raw[keyTopology]
	if !found {
		// multifile config
		topologyConf.topologyRaw = topologyConf.raw
		return nil
	}

	var ok bool
	if topologyConf.topologyRaw, ok = topologyConfRaw.(RawConfType); !ok {
		return fmt.Errorf("Clusterwide config %q value isn't a map", keyTopology)
	}

	return nil
}

func (topologyConf *TopologyConfType) setInstancesConf() error {
	instancesConfRaw, found := topologyConf.topologyRaw[keyInstances]
	if !found {
		return fmt.Errorf("Topology config doesn't contain %q key", keyInstances)
	}

	instancesConfRawMap, ok := instancesConfRaw.(RawConfType)
	if !ok {
		return fmt.Errorf("%q value isn't a map", keyInstances)
	}

	topologyConf.instancesRaw = instancesConfRawMap
	topologyConf.Instances = make(map[string]*InstanceConfType)

	for instanceUUIDRaw, instanceConfRaw := range instancesConfRawMap {
		instanceUUID, ok := instanceUUIDRaw.(string)
		if !ok {
			return fmt.Errorf("Instance UUID isn't a string")
		}

		var instanceConf InstanceConfType

		switch conf := instanceConfRaw.(type) {
		case string:
			if conf != expelledState {
				return fmt.Errorf("Instance %s is in the unknown state %s", instanceUUID, conf)
			}
			instanceConf.IsExpelled = true
		case RawConfType:
			isDisabled, ok := conf[keyInstanceDisabled]
			if !ok {
				return fmt.Errorf("Instance %s config doesn't contain %q key", instanceUUID, keyInstanceDisabled)
			}
			instanceConf.IsDisabled, ok = isDisabled.(bool)
			if !ok {
				return fmt.Errorf("Instance %s has %q that isn't a bool", instanceUUID, keyInstanceDisabled)
			}

			advertiseURI, ok := conf[keyInstanceAdvertiseURI]
			if !ok {
				return fmt.Errorf("Instance %s config doesn't contain %q key", instanceUUID, keyInstanceAdvertiseURI)
			}
			instanceConf.AdvertiseURI, ok = advertiseURI.(string)
			if !ok {
				return fmt.Errorf("Instance %s has %q that isn't a string", instanceUUID, keyInstanceAdvertiseURI)
			}

			replicasetUUID, ok := conf[keyInstanceReplicasetUUID]
			if !ok {
				return fmt.Errorf("Instance %s config doesn't contain %q key", instanceUUID, keyInstanceReplicasetUUID)
			}
			instanceConf.ReplicasetUUID, ok = replicasetUUID.(string)
			if !ok {
				return fmt.Errorf("Instance %s has %q that isn't a string", instanceUUID, keyInstanceReplicasetUUID)
			}

			instanceConf.Raw = conf
		default:
			return fmt.Errorf("Instance %s config isn't a map or a string", instanceUUID)
		}

		topologyConf.Instances[instanceUUID] = &instanceConf
	}

	return nil
}

func (topologyConf *TopologyConfType) setReplicasetsConf() error {
	replicasetsConfRaw, found := topologyConf.topologyRaw[keyReplicasets]
	if !found {
		return fmt.Errorf("Topology config doesn't contain %q key", keyReplicasets)
	}

	replicasetsConfRawMap, ok := replicasetsConfRaw.(RawConfType)
	if !ok {
		return fmt.Errorf("%q value isn't a map", keyReplicasets)
	}

	topologyConf.replicasetsRaw = replicasetsConfRawMap
	topologyConf.Replicasets = make(map[string]*ReplicasetConfType)

	for replicasetUUIDRaw, replicasetConfRaw := range replicasetsConfRawMap {
		replicasetUUID, ok := replicasetUUIDRaw.(string)
		if !ok {
			return fmt.Errorf("Replicaset UUID isn't a string")
		}

		var replicasetConf ReplicasetConfType

		switch conf := replicasetConfRaw.(type) {
		case RawConfType:
			replicasetConf.Raw = conf

			// alias
			aliasRaw, ok := conf[keyReplicasetAlias]
			if !ok {
				return fmt.Errorf("Replicaset %s config doesn't contain %q key", replicasetUUID, keyReplicasetAlias)
			}

			alias, ok := aliasRaw.(string)
			if !ok {
				return fmt.Errorf("Replicaset %q field isn't a string", keyReplicasetAlias)
			}

			if alias != unnamedReplicasetAlias {
				replicasetConf.Alias = alias
			}

			// roles
			rolesRaw, ok := conf[keyReplicasetRoles]
			if !ok {
				return fmt.Errorf("Replicaset %s config doesn't contain %q key", replicasetUUID, keyReplicasetRoles)
			}

			rolesRawConf, ok := rolesRaw.(RawConfType)
			if !ok {
				return fmt.Errorf("Replicaset %s config %q field isn't a map", replicasetUUID, keyReplicasetRoles)
			}

			for roleRaw := range rolesRawConf {
				role, ok := roleRaw.(string)
				if !ok {
					return fmt.Errorf("Replicaset %q map key %v isn't a string", replicasetUUID, roleRaw)
				}
				replicasetConf.Roles = append(replicasetConf.Roles, role)
			}

			// leaders
			leadersRaw, ok := conf[keyReplicasetLeaders]
			if !ok {
				return fmt.Errorf("Replicaset %s config doesn't contain %q key", replicasetUUID, keyReplicasetLeaders)
			}

			switch leadersConverted := leadersRaw.(type) {
			case string:
				replicasetConf.LeadersIsString = true
				replicasetConf.Leaders = append(replicasetConf.Leaders, leadersConverted)
			case []interface{}:
				leaders, err := common.ConvertToStringsSlice(leadersConverted)
				if err != nil {
					return fmt.Errorf("Replicaset %s %q field isn't a list of strings: %s", replicasetUUID, keyReplicasetLeaders, err)
				}

				replicasetConf.Leaders = leaders
			default:
				return fmt.Errorf("Replicaset %s %q field isn't a string or list of strings", replicasetUUID, keyReplicasetLeaders)
			}

			// instances
			replicasetConf.Instances = make([]string, 0)

			for instanceUUID, instanceConf := range topologyConf.Instances {
				if instanceConf.ReplicasetUUID == replicasetUUID {
					replicasetConf.Instances = append(replicasetConf.Instances, instanceUUID)
				}
			}

			sort.Sort(sort.StringSlice(replicasetConf.Instances))

		default:
			return fmt.Errorf("Replicaset %s config isn't a map", replicasetUUID)
		}

		topologyConf.Replicasets[replicasetUUID] = &replicasetConf
	}

	return nil
}

func (topologyConf *TopologyConfType) MarshalContent() ([]byte, error) {
	content, err := yaml.Marshal(topologyConf.raw)
	if err != nil {
		return nil, fmt.Errorf("Failed to YAML encode: %s", err)
	}

	return content, nil
}

// INSTANCES

func (topologyConf *TopologyConfType) GetOrderedInstaceUUIDs() []string {
	instanceUUIDs := make([]string, len(topologyConf.Instances))

	i := 0
	for instanceUUID := range topologyConf.Instances {
		instanceUUIDs[i] = instanceUUID
		i++
	}

	sort.Sort(sort.StringSlice(instanceUUIDs))

	return instanceUUIDs
}

func (topologyConf *TopologyConfType) SetInstanceURI(instanceUUID, newURI string) error {
	instanceConf, ok := topologyConf.Instances[instanceUUID]
	if !ok {
		return fmt.Errorf("Instance %s isn't found in cluster", instanceUUID)
	}

	if instanceConf.IsExpelled {
		return fmt.Errorf("Instance %s is expelled", instanceUUID)
	}

	instanceConf.AdvertiseURI = newURI
	instanceConf.Raw[keyInstanceAdvertiseURI] = newURI

	return nil
}

func (topologyConf *TopologyConfType) RemoveInstance(instanceUUID string) error {
	if _, ok := topologyConf.Instances[instanceUUID]; !ok {
		return fmt.Errorf("Instance %s isn't found in cluster", instanceUUID)
	}

	delete(topologyConf.Instances, instanceUUID)
	delete(topologyConf.instancesRaw, instanceUUID)

	return nil
}

// REPLICASETS

func (topologyConf *TopologyConfType) GetOrderedReplicasetUUIDs() []string {
	replicasetUUIDs := make([]string, len(topologyConf.Replicasets))

	i := 0
	for replicasetUUID := range topologyConf.Replicasets {
		replicasetUUIDs[i] = replicasetUUID
		i++
	}

	sort.Sort(sort.StringSlice(replicasetUUIDs))

	return replicasetUUIDs
}

func (topologyConf *TopologyConfType) RemoveReplicaset(replicasetUUID string) error {
	if _, ok := topologyConf.Replicasets[replicasetUUID]; !ok {
		return fmt.Errorf("Replicaset %s isn't found in cluster", replicasetUUID)
	}

	delete(topologyConf.Replicasets, replicasetUUID)
	delete(topologyConf.replicasetsRaw, replicasetUUID)

	return nil
}

func (replicasetConf *ReplicasetConfType) SetInstances(newInstances []string) {
	replicasetConf.Instances = newInstances
}

func (replicasetConf *ReplicasetConfType) SetLeaders(newLeaders []string) {
	replicasetConf.Leaders = newLeaders
	if replicasetConf.LeadersIsString {
		if len(newLeaders) > 0 {
			replicasetConf.Raw[keyReplicasetLeaders] = newLeaders[0]
		} else {
			delete(replicasetConf.Raw, keyReplicasetLeaders)
		}
	} else {
		replicasetConf.Raw[keyReplicasetLeaders] = newLeaders
	}
}

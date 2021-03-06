package repair

import (
	"fmt"

	"github.com/tarantool/cartridge-cli/cli/common"
	"github.com/tarantool/cartridge-cli/cli/context"
)

func patchConfRemoveInstance(topologyConf *TopologyConfType, ctx *context.Ctx) ([]common.ResultMessage, error) {
	return patchConf(removeInstance, topologyConf, ctx)
}

func removeInstance(topologyConf *TopologyConfType, ctx *context.Ctx) error {
	instanceUUID := ctx.Repair.RemoveInstanceUUID

	instanceConf, ok := topologyConf.Instances[instanceUUID]
	if !ok {
		return fmt.Errorf("Instance %s isn't found in cluster", instanceUUID)
	}

	if !instanceConf.IsExpelled {
		replicasetUUID := instanceConf.ReplicasetUUID
		replicasetConf, ok := topologyConf.Replicasets[replicasetUUID]

		if ok {
			leaderIndex := common.StringsSliceElemIndex(replicasetConf.Leaders, instanceUUID)
			if leaderIndex != -1 {
				replicasetConf.SetLeaders(common.RemoveFromStringSlice(replicasetConf.Leaders, leaderIndex))
			}

			instanceIndex := common.StringsSliceElemIndex(replicasetConf.Instances, instanceUUID)
			if instanceIndex != -1 {
				replicasetConf.SetInstances(common.RemoveFromStringSlice(replicasetConf.Instances, instanceIndex))
			}

			if len(replicasetConf.Leaders) == 0 {
				if len(replicasetConf.Instances) > 0 {
					replicasetConf.SetLeaders(append(replicasetConf.Leaders, replicasetConf.Instances[0]))
				}
			}

			if len(replicasetConf.Instances) == 0 {
				if err := topologyConf.RemoveReplicaset(replicasetUUID); err != nil {
					return fmt.Errorf("Failed to remove replicaset %s from config: %s", replicasetUUID, err)
				}
			}
		}
	}

	if err := topologyConf.RemoveInstance(instanceUUID); err != nil {
		return fmt.Errorf("Failed to remove instance %s from config: %s", instanceUUID, err)
	}

	return nil
}

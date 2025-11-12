package topo

import "sort"

// DeviceInfo carries scheduling metadata for a device.
type DeviceInfo struct {
	ID        int
	Island    string
	Bandwidth int // GB/s to peers within the chosen set
}

// NodeInfo bundles devices for a node (placeholder for future logic).
type NodeInfo struct {
	Name    string
	Devices []DeviceInfo
}

// ScoreContiguousSameIsland returns a score and contiguous pick that share the island.
func ScoreContiguousSameIsland(devs []DeviceInfo, count int) (score int, pick []int) {
	if len(devs) < count {
		return -1, nil
	}
	sort.Slice(devs, func(i, j int) bool { return devs[i].ID < devs[j].ID })

	bestScore := -1
	var bestPick []int
	for i := 0; i+count <= len(devs); i++ {
		island := devs[i].Island
		ok := true
		for k := 1; k < count; k++ {
			if devs[i+k].ID != devs[i].ID+k || devs[i+k].Island != island {
				ok = false
				break
			}
		}
		if ok {
			score := 1000 + devs[i].Bandwidth
			candidate := ids(devs[i : i+count])
			if score > bestScore {
				bestScore = score
				bestPick = candidate
			}
		}
	}
	return bestScore, bestPick
}

func ids(devs []DeviceInfo) []int {
	out := make([]int, len(devs))
	for i, d := range devs {
		out[i] = d.ID
	}
	return out
}

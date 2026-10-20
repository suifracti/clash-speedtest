package profiles

import "sort"

type SubscriptionNode struct {
	Identity string `json:"identity"`
	Name     string `json:"name"`
}
type SubscriptionNodeVersions struct {
	Previous []SubscriptionNode `json:"previous"`
	Current  []SubscriptionNode `json:"current"`
}
type NodeRename struct {
	From string `json:"from"`
	To   string `json:"to"`
}
type SubscriptionNodeChanges struct {
	Added   []string     `json:"added"`
	Removed []string     `json:"removed"`
	Renamed []NodeRename `json:"renamed"`
}

func (v *SubscriptionNodeVersions) Changes() *SubscriptionNodeChanges {
	if v == nil || v.Previous == nil {
		return nil
	}
	changes := &SubscriptionNodeChanges{Added: []string{}, Removed: []string{}, Renamed: []NodeRename{}}
	old := map[string][]string{}
	current := map[string][]string{}
	for _, n := range v.Previous {
		old[n.Identity] = append(old[n.Identity], n.Name)
	}
	for _, n := range v.Current {
		current[n.Identity] = append(current[n.Identity], n.Name)
	}
	identities := map[string]bool{}
	for id := range old {
		identities[id] = true
	}
	for id := range current {
		identities[id] = true
	}
	for id := range identities {
		before, after := old[id], current[id]
		// Match unchanged names first, including duplicate endpoint aliases.
		for i := 0; i < len(before); {
			found := -1
			for j, name := range after {
				if name == before[i] {
					found = j
					break
				}
			}
			if found < 0 {
				i++
				continue
			}
			before = append(before[:i], before[i+1:]...)
			after = append(after[:found], after[found+1:]...)
		}
		sort.Strings(before)
		sort.Strings(after)
		paired := min(len(before), len(after))
		for i := 0; i < paired; i++ {
			changes.Renamed = append(changes.Renamed, NodeRename{From: before[i], To: after[i]})
		}
		changes.Removed = append(changes.Removed, before[paired:]...)
		changes.Added = append(changes.Added, after[paired:]...)
	}
	sort.Strings(changes.Added)
	sort.Strings(changes.Removed)
	sort.Slice(changes.Renamed, func(i, j int) bool { return changes.Renamed[i].From < changes.Renamed[j].From })
	return changes
}

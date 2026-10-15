package graph

func HasCycle(nodeIDs []string, edges [][2]string) bool {
	out := map[string][]string{}
	for _, id := range nodeIDs {
		out[id] = nil
	}
	for _, e := range edges {
		if e[0] == e[1] {
			return true
		}
		if _, ok := out[e[0]]; !ok {
			return true
		}
		if _, ok := out[e[1]]; !ok {
			return true
		}
		out[e[0]] = append(out[e[0]], e[1])
	}
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := map[string]int{}
	var dfs func(string) bool
	dfs = func(id string) bool {
		if color[id] == gray {
			return true
		}
		if color[id] == black {
			return false
		}
		color[id] = gray
		for _, next := range out[id] {
			if dfs(next) {
				return true
			}
		}
		color[id] = black
		return false
	}
	for _, id := range nodeIDs {
		if dfs(id) {
			return true
		}
	}
	return false
}

func Unlocked(requiredPredecessors []string, done map[string]bool) bool {
	for _, id := range requiredPredecessors {
		if !done[id] {
			return false
		}
	}
	return true
}

package exporter

import "github.com/opsgenie/kubernetes-event-exporter/pkg/kube"

// Route allows using rules to drop events or match events to specific receivers.
// It also allows using routes recursively for complex route building to fit
// most of the needs
type Route struct {
	Drop   []Rule
	Match  []Rule
	Routes []Route
}

func (r *Route) FindMatchedRules(ev *kube.EnhancedEvent) (bool, []Rule) {
	var matchedRules []Rule

	// First determine whether we will drop the event: If any of the drop is matched, we break the loop
	for _, v := range r.Drop {
		if v.MatchesEvent(ev) {
			return false, matchedRules
		}
	}

	// It has match rules, it should go to the matchers
	matchesAll := true
	for _, rule := range r.Match {
		if rule.MatchesEvent(ev) {
			if rule.Receiver != "" {
				matchedRules = append(matchedRules, rule)
				// Send the event down the hole
			}
		} else {
			matchesAll = false
		}
	}

	return matchesAll, matchedRules
}

func (r *Route) ProcessEvent(ev *kube.EnhancedEvent, registry ReceiverRegistry) {
	matchesAll, matchedRules := r.FindMatchedRules(ev)

	for _, rule := range matchedRules {
		registry.SendEvent(rule.Receiver, ev)
	}

	// If all matches are satisfied, we can send them down to the rabbit hole
	if matchesAll {
		for _, subRoute := range r.Routes {
			subRoute.ProcessEvent(ev, registry)
		}
	}
}

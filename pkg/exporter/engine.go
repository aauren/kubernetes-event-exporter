package exporter

import (
	"github.com/opsgenie/kubernetes-event-exporter/pkg/kube"
	"github.com/rs/zerolog/log"
	"reflect"
)

// Engine is responsible for initializing the receivers from sinks
type Engine struct {
	Route    Route
	Registry ReceiverRegistry
}

func NewEngine(config *Config, registry ReceiverRegistry) *Engine {
	for _, v := range config.Receivers {
		sink, err := v.GetSink()
		if err != nil {
			log.Fatal().Err(err).Str("name", v.Name).Msg("Cannot initialize sink")
		}

		log.Info().
			Str("name", v.Name).
			Str("type", reflect.TypeOf(sink).String()).
			Msg("Registering sink")

		registry.Register(v.Name, sink)
	}

	return &Engine{
		Route:    config.Route,
		Registry: registry,
	}
}

// OnEvent does not care whether event is add or update. Prior filtering should be done in the controller/watcher
func (e *Engine) OnEvent(event *kube.EnhancedEvent) {
	e.Route.ProcessEvent(event, e.Registry)
}

// OnCheck checks event against the current config to see if we should process this event
func (e *Engine) OnCheck(event *kube.EnhancedEvent) bool {
	routes := []Route{e.Route}
	for _, subRoute := range e.Route.Routes {
		routes = append(routes, subRoute)
	}

	for _, route := range routes {
		matchesAll, matchedRules := route.FindMatchedRules(event)
		// If at any point we don't match all, then don't continue (this could be because drop condition was met or
		// because one of the match rules specifically excluded this event)
		if !matchesAll {
			return false
		}
		if matchedRules != nil && len(matchedRules) > 0 {
			return true
		}
	}
	return false
}

// Stop stops all registered sinks
func (e *Engine) Stop() {
	log.Info().Msg("Closing sinks")
	e.Registry.Close()
	log.Info().Msg("All sinks closed")
}

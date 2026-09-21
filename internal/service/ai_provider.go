package service

import (
	"context"

	"github.com/meridian-labs/meridian/internal/ai"
)

const commandAIProviderID = "command-producer"

// commandAIProvider is the first built-in implementation of the AI provider
// capability. The workflow remains the owner of persistence and policy; this
// provider owns only the producer invocation boundary. A future RPC provider
// can implement ai.Provider without changing AiWorkflow.
type commandAIProvider struct {
	workflow *AiWorkflow
}

func (provider commandAIProvider) Descriptor() ai.Descriptor {
	return ai.Descriptor{ID: commandAIProviderID, Version: "1"}
}

func (provider commandAIProvider) Generate(ctx context.Context, request ai.Request) (ai.Result, error) {
	profile := producerProfileFromConfig(request.Config)
	jobContext := AiGenerationJobContext{
		Kind: request.Kind, Name: request.Name, Hint: request.Hint,
		RefType: request.RefType, RefName: request.Ref,
		ScopeType: request.Metadata["scopeType"], ScopeKey: request.Metadata["scopeKey"],
		ServiceRoot: request.Metadata["serviceRoot"],
	}
	content, manifest, stage, code, err := provider.workflow.runProducer(ctx, profile, jobContext)
	return ai.Result{Content: []byte(content), ContentType: aiRevisionContentTypeDefault, Manifest: manifest, Stage: stage, ErrorCode: code}, err
}

func commandProviderConfig(profile ProducerProfile) map[string]any {
	return map[string]any{
		"name": profile.Name, "executable": profile.Executable, "args": profile.Args,
		"timeoutSec": profile.TimeoutSec, "network": profile.Network,
	}
}

func producerProfileFromConfig(config map[string]any) ProducerProfile {
	profile := ProducerProfile{}
	profile.Name, _ = config["name"].(string)
	profile.Executable, _ = config["executable"].(string)
	profile.Network, _ = config["network"].(string)
	if timeout, ok := config["timeoutSec"].(int); ok {
		profile.TimeoutSec = timeout
	} else if timeout, ok := config["timeoutSec"].(float64); ok {
		profile.TimeoutSec = int(timeout)
	}
	if args, ok := config["args"].([]string); ok {
		profile.Args = append([]string(nil), args...)
	} else if values, ok := config["args"].([]any); ok {
		for _, value := range values {
			if argument, ok := value.(string); ok {
				profile.Args = append(profile.Args, argument)
			}
		}
	}
	return profile
}

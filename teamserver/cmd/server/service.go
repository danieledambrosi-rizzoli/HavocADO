package server

import (
	"Havoc/pkg/agent"
	"Havoc/pkg/logger"
	"fmt"
)

func (t *Teamserver) ServiceAgent(MagicValue int) agent.ServiceAgentInterface {
	// The profile may not declare a Service block; an unknown magic value must
	// then be a parse failure, not a nil dereference.
	if t.Service == nil {
		logger.Debug("Service agent not found")
		return nil
	}

	for _, agentService := range t.Service.Agents {
		if agentService.MagicValue == fmt.Sprintf("0x%x", MagicValue) {
			return agentService
		}
	}

	logger.Debug("Service agent not found")
	return nil
}

func (t *Teamserver) ServiceAgentExist(MagicValue int) bool {
	if t.Service == nil {
		logger.Debug("Service agent not found")
		return false
	}

	for _, agentService := range t.Service.Agents {
		if agentService.MagicValue == fmt.Sprintf("0x%x", MagicValue) {
			return true
		}
	}

	logger.Debug("Service agent not found")
	return false
}

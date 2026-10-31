package node

func (ac *AgentConn) SetServicesVersion(version int) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	ac.servicesVersion = version
}

func (ac *AgentConn) ServicesVersion() int {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	return ac.servicesVersion
}

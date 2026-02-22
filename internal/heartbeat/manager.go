package heartbeat

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

type HeartbeatStatus string

const (
	StatusHealthy   HeartbeatStatus = "healthy"
	StatusUnhealthy HeartbeatStatus = "unhealthy"
	StatusUnknown   HeartbeatStatus = "unknown"
)

type HealthCheckFunc func(ctx context.Context) error

type ComponentHealth struct {
	Name      string          `json:"name"`
	Status    HeartbeatStatus `json:"status"`
	LastCheck time.Time       `json:"lastCheck"`
	Error     string          `json:"error,omitempty"`
	Latency   time.Duration   `json:"latency"`
}

type HeartbeatResult struct {
	Timestamp time.Time        `json:"timestamp"`
	Status    HeartbeatStatus  `json:"status"`
	Uptime    time.Duration    `json:"uptime"`
	Components []ComponentHealth `json:"components"`
}

type HeartbeatConfig struct {
	Interval         time.Duration
	Timeout          time.Duration
	MaxFailures      int
	RecoveryInterval time.Duration
}

type HeartbeatManager struct {
	config       HeartbeatConfig
	checks       map[string]HealthCheckFunc
	status       map[string]*ComponentHealth
	startTime    time.Time
	failureCount map[string]int
	stopChan     chan struct{}
	mu           sync.RWMutex
	running      bool
}

var globalManager *HeartbeatManager
var managerOnce sync.Once

func GetHeartbeatManager() *HeartbeatManager {
	managerOnce.Do(func() {
		globalManager = &HeartbeatManager{
			config: HeartbeatConfig{
				Interval:         30 * time.Second,
				Timeout:          10 * time.Second,
				MaxFailures:      3,
				RecoveryInterval: 60 * time.Second,
			},
			checks:       make(map[string]HealthCheckFunc),
			status:       make(map[string]*ComponentHealth),
			failureCount: make(map[string]int),
			stopChan:     make(chan struct{}),
			startTime:    time.Now(),
		}
	})
	return globalManager
}

func (m *HeartbeatManager) RegisterCheck(name string, check HealthCheckFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.checks[name] = check
	m.status[name] = &ComponentHealth{
		Name:   name,
		Status: StatusUnknown,
	}
}

func (m *HeartbeatManager) UnregisterCheck(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.checks, name)
	delete(m.status, name)
	delete(m.failureCount, name)
}

func (m *HeartbeatManager) Start() {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return
	}
	m.running = true
	m.mu.Unlock()

	go m.runLoop()
}

func (m *HeartbeatManager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return
	}

	close(m.stopChan)
	m.running = false
}

func (m *HeartbeatManager) runLoop() {
	ticker := time.NewTicker(m.config.Interval)
	defer ticker.Stop()

	m.runChecks()

	for {
		select {
		case <-m.stopChan:
			return
		case <-ticker.C:
			m.runChecks()
		}
	}
}

func (m *HeartbeatManager) runChecks() {
	m.mu.RLock()
	checks := make(map[string]HealthCheckFunc, len(m.checks))
	for k, v := range m.checks {
		checks[k] = v
	}
	m.mu.RUnlock()

	var wg sync.WaitGroup
	for name, check := range checks {
		wg.Add(1)
		go func(name string, check HealthCheckFunc) {
			defer wg.Done()
			m.runSingleCheck(name, check)
		}(name, check)
	}
	wg.Wait()
}

func (m *HeartbeatManager) runSingleCheck(name string, check HealthCheckFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), m.config.Timeout)
	defer cancel()

	start := time.Now()
	err := check(ctx)
	latency := time.Since(start)

	m.mu.Lock()
	defer m.mu.Unlock()

	health := &ComponentHealth{
		Name:      name,
		LastCheck: time.Now(),
		Latency:   latency,
	}

	if err != nil {
		health.Status = StatusUnhealthy
		health.Error = err.Error()
		m.failureCount[name]++
	} else {
		health.Status = StatusHealthy
		health.Error = ""
		m.failureCount[name] = 0
	}

	m.status[name] = health
}

func (m *HeartbeatManager) GetStatus() *HeartbeatResult {
	m.mu.RLock()
	defer m.mu.RUnlock()

	components := make([]ComponentHealth, 0, len(m.status))
	overallStatus := StatusHealthy

	for _, health := range m.status {
		components = append(components, *health)
		if health.Status == StatusUnhealthy {
			overallStatus = StatusUnhealthy
		}
	}

	if len(components) == 0 {
		overallStatus = StatusUnknown
	}

	return &HeartbeatResult{
		Timestamp:  time.Now(),
		Status:     overallStatus,
		Uptime:     time.Since(m.startTime),
		Components: components,
	}
}

func (m *HeartbeatManager) GetComponentStatus(name string) *ComponentHealth {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if health, ok := m.status[name]; ok {
		return health
	}
	return nil
}

func (m *HeartbeatManager) IsHealthy() bool {
	result := m.GetStatus()
	return result.Status == StatusHealthy
}

func (m *HeartbeatManager) GetUptime() time.Duration {
	return time.Since(m.startTime)
}

func (m *HeartbeatManager) SetConfig(config HeartbeatConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config = config
}

type HeartbeatHook func(result *HeartbeatResult)

type HookManager struct {
	hooks []HeartbeatHook
	mu    sync.RWMutex
}

var globalHookManager *HookManager
var hookManagerOnce sync.Once

func GetHookManager() *HookManager {
	hookManagerOnce.Do(func() {
		globalHookManager = &HookManager{
			hooks: make([]HeartbeatHook, 0),
		}
	})
	return globalHookManager
}

func (h *HookManager) Register(hook HeartbeatHook) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.hooks = append(h.hooks, hook)
}

func (h *HookManager) Trigger(result *HeartbeatResult) {
	h.mu.RLock()
	hooks := make([]HeartbeatHook, len(h.hooks))
	copy(hooks, h.hooks)
	h.mu.RUnlock()

	for _, hook := range hooks {
		go func(hook HeartbeatHook) {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("Heartbeat hook panic: %v", r)
				}
			}()
			hook(result)
		}(hook)
	}
}

type AlertConfig struct {
	Enabled       bool
	OnUnhealthy   bool
	OnRecovery    bool
	WebhookURL    string
	AlertCooldown time.Duration
}

type AlertManager struct {
	config       AlertConfig
	lastAlert    map[string]time.Time
	mu           sync.RWMutex
}

var globalAlertManager *AlertManager
var alertManagerOnce sync.Once

func GetAlertManager() *AlertManager {
	alertManagerOnce.Do(func() {
		globalAlertManager = &AlertManager{
			config: AlertConfig{
				Enabled:       false,
				OnUnhealthy:   true,
				OnRecovery:    true,
				AlertCooldown: 5 * time.Minute,
			},
			lastAlert: make(map[string]time.Time),
		}
	})
	return globalAlertManager
}

func (a *AlertManager) SetConfig(config AlertConfig) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.config = config
}

func (a *AlertManager) ShouldAlert(componentName string, isRecovery bool) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if !a.config.Enabled {
		return false
	}

	if isRecovery && !a.config.OnRecovery {
		return false
	}

	if !isRecovery && !a.config.OnUnhealthy {
		return false
	}

	lastAlert, exists := a.lastAlert[componentName]
	if !exists {
		return true
	}

	return time.Since(lastAlert) > a.config.AlertCooldown
}

func (a *AlertManager) RecordAlert(componentName string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.lastAlert[componentName] = time.Now()
}

func (a *AlertManager) SendAlert(componentName string, status HeartbeatStatus, message string) error {
	if !a.ShouldAlert(componentName, status == StatusHealthy) {
		return nil
	}

	log.Printf("[ALERT] Component %s: %s - %s", componentName, status, message)
	a.RecordAlert(componentName)

	return nil
}

func QuickHealthCheck(ctx context.Context, checkFuncs ...HealthCheckFunc) *HeartbeatResult {
	start := time.Now()
	components := make([]ComponentHealth, 0, len(checkFuncs))
	overallStatus := StatusHealthy

	for i, check := range checkFuncs {
		name := fmt.Sprintf("check-%d", i)
		checkStart := time.Now()
		err := check(ctx)
		latency := time.Since(checkStart)

		health := ComponentHealth{
			Name:      name,
			LastCheck: time.Now(),
			Latency:   latency,
		}

		if err != nil {
			health.Status = StatusUnhealthy
			health.Error = err.Error()
			overallStatus = StatusUnhealthy
		} else {
			health.Status = StatusHealthy
		}

		components = append(components, health)
	}

	return &HeartbeatResult{
		Timestamp:  time.Now(),
		Status:     overallStatus,
		Uptime:     time.Since(start),
		Components: components,
	}
}

func SimpleHealthCheck(name string, check func() error) HealthCheckFunc {
	return func(ctx context.Context) error {
		resultChan := make(chan error, 1)
		go func() {
			resultChan <- check()
		}()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-resultChan:
			return err
		}
	}
}

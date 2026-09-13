package tools

import (
	"sync"

	"github.com/anurag/nexus/internal/config"
	"github.com/anurag/nexus/internal/confluence"
	"github.com/anurag/nexus/internal/figma"
	"github.com/anurag/nexus/internal/jira"
	"github.com/anurag/nexus/internal/store"
	"github.com/anurag/nexus/model"
)

type Context struct {
	mu         sync.RWMutex
	Store      *store.Store
	Services   map[string]*model.ServiceIndex
	Graph      *model.Graph
	Jira       *jira.Client
	Confluence *confluence.Client
	Figma      *figma.Client
	AgentCfg   config.AgentConfig
	Cfg        *config.Config
}

func (c *Context) SwapIndex(services map[string]*model.ServiceIndex, graph *model.Graph) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Services = services
	c.Graph = graph
}

func (c *Context) RLock()   { c.mu.RLock() }
func (c *Context) RUnlock() { c.mu.RUnlock() }

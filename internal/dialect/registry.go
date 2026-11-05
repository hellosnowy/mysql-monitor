package dialect

import (
	"fmt"
	"sync"

	"sql-monitor/internal/model"
)

// Registry 方言插件注册表
type Registry struct {
	mu      sync.RWMutex
	plugins map[model.Vendor]Plugin
	order   []model.Vendor
}

// NewRegistry 创建空注册表
func NewRegistry() *Registry {
	return &Registry{plugins: make(map[model.Vendor]Plugin)}
}

// Register 注册一个方言插件
func (r *Registry) Register(p Plugin) error {
	if p == nil {
		return fmt.Errorf("不能注册空的方言插件")
	}
	info := p.Info()
	vendor := model.NormalizeVendor(info.Vendor)
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.plugins[vendor]; exists {
		return fmt.Errorf("方言插件 %s 已注册", vendor)
	}
	r.plugins[vendor] = p
	r.order = append(r.order, vendor)
	return nil
}

// MustRegister 注册方言插件，重复或为空时 panic（用于内置注册）
func (r *Registry) MustRegister(p Plugin) {
	if err := r.Register(p); err != nil {
		panic(err)
	}
}

// Get 根据厂商获取插件，空厂商归一化为 MySQL
func (r *Registry) Get(vendor model.Vendor) (Plugin, error) {
	vendor = model.NormalizeVendor(vendor)
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.plugins[vendor]
	if !ok {
		return nil, fmt.Errorf("不支持的数据库类型: %s", vendor)
	}
	return p, nil
}

// List 返回所有已注册方言的信息，按注册顺序
func (r *Registry) List() []DriverInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]DriverInfo, 0, len(r.order))
	for _, v := range r.order {
		list = append(list, r.plugins[v].Info())
	}
	return list
}

package builtin

import (
	"testing"

	"sql-monitor/internal/model"
)

// TestBuiltinRegistryHasAllVendors 验证三个内置方言均已注册
func TestBuiltinRegistryHasAllVendors(t *testing.T) {
	r := NewRegistry()
	infos := r.List()
	if len(infos) != 3 {
		t.Fatalf("内置方言数量 = %d, want 3", len(infos))
	}

	for _, vendor := range []model.Vendor{model.VendorMySQL, model.VendorPostgres, model.VendorSQLServer} {
		p, err := r.Get(vendor)
		if err != nil {
			t.Fatalf("获取方言 %s 失败: %v", vendor, err)
		}
		if !p.Info().BuiltIn {
			t.Errorf("方言 %s 未标记为内置驱动", vendor)
		}
		if p.Emitter() == nil {
			t.Errorf("方言 %s 未提供 Emitter", vendor)
		}
	}

	// 空厂商回退 MySQL
	if _, err := r.Get(""); err != nil {
		t.Errorf("空厂商应回退 MySQL: %v", err)
	}
}

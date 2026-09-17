package filter

import (
	"testing"

	"mysql-monitor/internal/model"
)

func TestTableFilter(t *testing.T) {
	tests := []struct {
		name      string
		config    model.TableFilterConfig
		tableName string
		want      bool
	}{
		{
			name: "默认全量（白名单与黑名单均为空）",
			config: model.TableFilterConfig{
				IncludeTables: nil,
				ExcludeTables: nil,
			},
			tableName: "t_order",
			want:      true,
		},
		{
			name: "白名单精确匹配成功",
			config: model.TableFilterConfig{
				IncludeTables: []string{"t_user", "t_order"},
			},
			tableName: "t_user",
			want:      true,
		},
		{
			name: "白名单精确匹配失败",
			config: model.TableFilterConfig{
				IncludeTables: []string{"t_user", "t_order"},
			},
			tableName: "t_product",
			want:      false,
		},
		{
			name: "通配符白名单匹配",
			config: model.TableFilterConfig{
				IncludeTables: []string{"sys_*", "order_*"},
			},
			tableName: "order_item",
			want:      true,
		},
		{
			name: "黑名单排除",
			config: model.TableFilterConfig{
				ExcludeTables: []string{"*_bak", "tmp_*"},
			},
			tableName: "order_bak",
			want:      false,
		},
		{
			name: "黑白名单冲突时黑名单优先",
			config: model.TableFilterConfig{
				IncludeTables: []string{"order_*"},
				ExcludeTables: []string{"*_bak"},
			},
			tableName: "order_bak",
			want:      false, // 命中黑名单，排除
		},
		{
			name: "正则匹配",
			config: model.TableFilterConfig{
				IncludeTables: []string{`^tbl_\d+$`},
			},
			tableName: "tbl_2024",
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := NewTableFilter(tt.config)
			got := f.ShouldInclude(tt.tableName)
			if got != tt.want {
				t.Errorf("ShouldInclude(%q) = %v, want %v", tt.tableName, got, tt.want)
			}
		})
	}
}

func TestFilterTables(t *testing.T) {
	cfg := model.TableFilterConfig{
		IncludeTables: []string{"user", "order_*"},
		ExcludeTables: []string{"*_history"},
	}
	f := NewTableFilter(cfg)

	all := []string{"user", "order_item", "order_history", "product", "setting"}
	got := f.FilterTables(all)
	want := []string{"user", "order_item"}

	if len(got) != len(want) {
		t.Fatalf("FilterTables got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("FilterTables[%d] = %s, want %s", i, got[i], want[i])
		}
	}
}

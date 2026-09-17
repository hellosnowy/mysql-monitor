package filter

import (
	"path/filepath"
	"regexp"
	"strings"

	"mysql-monitor/internal/model"
)

// TableFilter 数据表黑白名单过滤器
type TableFilter struct {
	config model.TableFilterConfig
}

// NewTableFilter 创建一个新的数据表黑白名单过滤器
func NewTableFilter(config model.TableFilterConfig) *TableFilter {
	return &TableFilter{
		config: config,
	}
}

// ShouldInclude 判断指定表名是否应该被纳入监控范围
// 规则：
// 1. 若匹配黑名单任一规则，则坚决排除（返回 false）；
// 2. 若白名单为空，则默认全部包含（返回 true）；
// 3. 若白名单不为空，则必须匹配白名单中至少一项才包含（返回 true），否则排除（返回 false）。
func (f *TableFilter) ShouldInclude(tableName string) bool {
	// 1. 优先校验黑名单
	for _, pattern := range f.config.ExcludeTables {
		if matchPattern(pattern, tableName) {
			return false
		}
	}

	// 2. 校验白名单
	if len(f.config.IncludeTables) == 0 {
		return true
	}

	for _, pattern := range f.config.IncludeTables {
		if matchPattern(pattern, tableName) {
			return true
		}
	}

	return false
}

// FilterTables 批量过滤表名列表，返回通过黑白名单筛选的表名清单
func (f *TableFilter) FilterTables(tables []string) []string {
	var result []string
	for _, t := range tables {
		if f.ShouldInclude(t) {
			result = append(result, t)
		}
	}
	return result
}

// matchPattern 统一的模式匹配算法：支持精确匹配、Glob 通配符与正则表达式
func matchPattern(pattern, target string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return false
	}

	// 1. 精确匹配（不区分大小写比较更符合 MySQL 常见习惯，统一转小写匹配）
	if strings.EqualFold(pattern, target) {
		return true
	}

	// 2. 如果 pattern 具备明显正则表达式特征（如以 ^ 开头或以 $ 结尾，或包含 \\d, ( 等）
	if strings.HasPrefix(pattern, "^") || strings.HasSuffix(pattern, "$") || strings.Contains(pattern, "\\") {
		if re, err := regexp.Compile("(?i)" + pattern); err == nil {
			return re.MatchString(target)
		}
	}

	// 3. Glob 通配符模式匹配（如 `t_*`, `*_bak`, `sys_?`）
	if matched, err := filepath.Match(strings.ToLower(pattern), strings.ToLower(target)); err == nil && matched {
		return true
	}

	return false
}

package sqlutil

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"mysql-monitor/internal/model"
)

// EscapeIdentifier 对 MySQL 标识符（表名、列名、索引名等）添加反引号包裹并转义内部反引号
func EscapeIdentifier(name string) string {
	clean := strings.ReplaceAll(name, "`", "``")
	return fmt.Sprintf("`%s`", clean)
}

// EscapeStringLiteral 对字符串字面量转义单引号、反斜杠、空字符、换行等
func EscapeStringLiteral(s string) string {
	var sb strings.Builder
	sb.Grow(len(s) + 8)
	sb.WriteByte('\'')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case 0:
			sb.WriteString(`\0`)
		case '\n':
			sb.WriteString(`\n`)
		case '\r':
			sb.WriteString(`\r`)
		case '\\':
			sb.WriteString(`\\`)
		case '\'':
			sb.WriteString(`\'`)
		case '"':
			sb.WriteString(`\"`)
		case '\032':
			sb.WriteString(`\Z`)
		default:
			sb.WriteByte(c)
		}
	}
	sb.WriteByte('\'')
	return sb.String()
}

// FormatSQLValue 将任意 Go 类型的值转换为 MySQL 合法的 SQL 字面量表示
func FormatSQLValue(val interface{}) string {
	if val == nil {
		return "NULL"
	}

	switch v := val.(type) {
	case json.Number:
		encoded, err := json.Marshal(v)
		if err != nil {
			return EscapeStringLiteral(v.String())
		}
		return string(encoded)
	case string:
		return EscapeStringLiteral(v)
	case []byte:
		// 二进制数据使用十六进制语法 0x...
		return fmt.Sprintf("0x%s", hex.EncodeToString(v))
	case bool:
		if v {
			return "1"
		}
		return "0"
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", v)
	case float32, float64:
		return fmt.Sprintf("%v", v)
	case time.Time:
		return fmt.Sprintf("'%s'", v.Format("2006-01-02 15:04:05"))
	default:
		// 处理字符串化兜底
		strVal := fmt.Sprintf("%v", v)
		return EscapeStringLiteral(strVal)
	}
}

// FormatColumnSpec 格式化单个列的完整定义语句（用于 ADD/MODIFY COLUMN）
func FormatColumnSpec(col *model.ColumnSchema) string {
	var sb strings.Builder
	sb.WriteString(EscapeIdentifier(col.Name))
	sb.WriteString(" ")
	sb.WriteString(col.ColumnType)

	if col.CharacterSetName != "" && col.CollationName != "" {
		sb.WriteString(fmt.Sprintf(" CHARACTER SET %s COLLATE %s", col.CharacterSetName, col.CollationName))
	}

	if strings.ToUpper(col.IsNullable) == "NO" {
		sb.WriteString(" NOT NULL")
	} else {
		sb.WriteString(" NULL")
	}

	if col.ColumnDefault != nil {
		defaultVal := *col.ColumnDefault
		upperDefault := strings.ToUpper(defaultVal)
		// 如果默认值是 CURRENT_TIMESTAMP 等内置函数或关键字，不需要包裹引号
		if upperDefault == "CURRENT_TIMESTAMP" || upperDefault == "NULL" || strings.HasPrefix(upperDefault, "CURRENT_TIMESTAMP(") {
			sb.WriteString(fmt.Sprintf(" DEFAULT %s", defaultVal))
		} else {
			sb.WriteString(fmt.Sprintf(" DEFAULT %s", EscapeStringLiteral(defaultVal)))
		}
	}

	if col.Extra != "" {
		sb.WriteString(" ")
		sb.WriteString(col.Extra)
	}

	if col.Comment != "" {
		sb.WriteString(fmt.Sprintf(" COMMENT %s", EscapeStringLiteral(col.Comment)))
	}

	return sb.String()
}

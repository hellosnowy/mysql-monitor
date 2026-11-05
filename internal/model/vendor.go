package model

// Vendor 数据库厂商标识
type Vendor string

const (
	// VendorMySQL MySQL
	VendorMySQL Vendor = "mysql"
	// VendorPostgres PostgreSQL
	VendorPostgres Vendor = "postgres"
	// VendorSQLServer Microsoft SQL Server
	VendorSQLServer Vendor = "sqlserver"
)

// NormalizeVendor 将空厂商归一化为 MySQL，兼容 mysql-monitor 老数据
func NormalizeVendor(v Vendor) Vendor {
	if v == "" {
		return VendorMySQL
	}
	return v
}

// DisplayName 返回厂商的展示名称
func (v Vendor) DisplayName() string {
	switch NormalizeVendor(v) {
	case VendorPostgres:
		return "PostgreSQL"
	case VendorSQLServer:
		return "SQL Server"
	default:
		return "MySQL"
	}
}

// DiffDirection 版本比对方向
type DiffDirection string

const (
	// DirectionUpgrade 由起始版本升级到目标版本 (from -> to)
	DirectionUpgrade DiffDirection = "upgrade"
	// DirectionRollback 由目标版本回滚到起始版本 (to -> from)
	DirectionRollback DiffDirection = "rollback"
)

// NormalizeDirection 将空方向归一化为升级
func NormalizeDirection(d DiffDirection) DiffDirection {
	if d == DirectionRollback {
		return DirectionRollback
	}
	return DirectionUpgrade
}

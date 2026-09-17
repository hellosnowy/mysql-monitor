package model

import "time"

// DDLDiffSummary DDL 结构层面的差异数量统计
type DDLDiffSummary struct {
	// AddedTables 新增表数量
	AddedTables int `json:"added_tables"`
	// DroppedTables 删除表数量
	DroppedTables int `json:"dropped_tables"`
	// ModifiedTables 结构发生变更的表数量
	ModifiedTables int `json:"modified_tables"`
	// AddedColumns 新增字段总数
	AddedColumns int `json:"added_columns"`
	// DroppedColumns 删除字段总数
	DroppedColumns int `json:"dropped_columns"`
	// ModifiedColumns 修改字段总数
	ModifiedColumns int `json:"modified_columns"`
	// AddedIndexes 新增索引总数
	AddedIndexes int `json:"added_indexes"`
	// DroppedIndexes 删除索引总数
	DroppedIndexes int `json:"dropped_indexes"`
}

// DMLDiffSummary DML 数据层面的变更行数统计
type DMLDiffSummary struct {
	// InsertCount 新增数据行数
	InsertCount int `json:"insert_count"`
	// UpdateCount 修改数据行数
	UpdateCount int `json:"update_count"`
	// DeleteCount 删除数据行数
	DeleteCount int `json:"delete_count"`
}

// DiffSummary 综合差异变更统计
type DiffSummary struct {
	// DDL 结构变动统计
	DDL DDLDiffSummary `json:"ddl"`
	// DML 数据变动统计
	DML DMLDiffSummary `json:"dml"`
}

// TableDDLDiff 单个数据表的 DDL 变动详情
type TableDDLDiff struct {
	// TableName 表名
	TableName string `json:"table_name"`
	// DiffType 变动主类型: "CREATE_TABLE", "DROP_TABLE", "ALTER_TABLE"
	DiffType string `json:"diff_type"`
	// Statements 针对该表生成的所有 DDL SQL 语句列表
	Statements []string `json:"statements"`
	// Details 人类可读的详细改动项（如：新增字段 age INT、修改字段 status 等）
	Details []string `json:"details"`
}

// TableDMLDiff 单个数据表的 DML 变动详情
type TableDMLDiff struct {
	// TableName 表名
	TableName string `json:"table_name"`
	// InsertStatements 新增数据的 INSERT 语句列表
	InsertStatements []string `json:"insert_statements"`
	// UpdateStatements 修改数据的 UPDATE 语句列表
	UpdateStatements []string `json:"update_statements"`
	// DeleteStatements 删除数据的 DELETE 语句列表
	DeleteStatements []string `json:"delete_statements"`
	// InsertCount 新增行数
	InsertCount int `json:"insert_count"`
	// UpdateCount 更新行数
	UpdateCount int `json:"update_count"`
	// DeleteCount 删除行数
	DeleteCount int `json:"delete_count"`
}

// DiffResult 任意两版本对比的完整输出结果
type DiffResult struct {
	// ConnectionID 所属数据库连接 ID
	ConnectionID string `json:"connection_id"`
	// FromVersionID 起始/基准版本号
	FromVersionID string `json:"from_version_id"`
	// ToVersionID 目标版本号
	ToVersionID string `json:"to_version_id"`
	// ComparedAt 比对生成时间
	ComparedAt time.Time `json:"compared_at"`
	// Summary 变动统计概览
	Summary DiffSummary `json:"summary"`
	// TableDDLDiffs 表结构差异详情列表
	TableDDLDiffs []*TableDDLDiff `json:"table_ddl_diffs"`
	// TableDMLDiffs 表数据差异详情列表
	TableDMLDiffs []*TableDMLDiff `json:"table_dml_diffs"`
	// DDLScript 纯 DDL 语句汇总脚本
	DDLScript string `json:"ddl_script"`
	// DMLScript 纯 DML 语句汇总脚本
	DMLScript string `json:"dml_script"`
	// FullScript 包含事务保护与版本说明的完整一键执行 SQL 脚本
	FullScript string `json:"full_script"`
}

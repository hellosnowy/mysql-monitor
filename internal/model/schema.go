package model

// ColumnSchema 单个数据表字段的结构元数据
type ColumnSchema struct {
	// Name 列名
	Name string `json:"name"`
	// OrdinalPosition 列在表中的位置顺序（从1开始）
	OrdinalPosition int `json:"ordinal_position"`
	// ColumnType 完整的列类型定义，如 varchar(255), int(11) unsigned, numeric(18,2)
	ColumnType string `json:"column_type"`
	// DataType 基础数据类型，如 varchar, int, datetime
	DataType string `json:"data_type"`
	// IsNullable 是否允许为 NULL ("YES" 或 "NO")
	IsNullable string `json:"is_nullable"`
	// ColumnDefault 字段默认值指针，nil 表示无默认值或默认为 NULL
	ColumnDefault *string `json:"column_default"`
	// Extra 扩展属性，如 auto_increment, on update CURRENT_TIMESTAMP
	Extra string `json:"extra"`
	// Comment 字段注释
	Comment string `json:"comment"`
	// CharacterSetName 字段字符集，如 utf8mb4
	CharacterSetName string `json:"character_set_name"`
	// CollationName 字段排序规则，如 utf8mb4_unicode_ci
	CollationName string `json:"collation_name"`
	// IsIdentity 是否为自增/IDENTITY 列
	IsIdentity bool `json:"is_identity,omitempty"`
	// IdentityGeneration PostgreSQL identity 生成方式: ALWAYS / BY DEFAULT
	IdentityGeneration string `json:"identity_generation,omitempty"`
	// IsGenerated 是否为生成列/计算列
	IsGenerated bool `json:"is_generated,omitempty"`
	// GenerationExpression 生成列表达式
	GenerationExpression string `json:"generation_expression,omitempty"`
	// DefaultConstraintName SQL Server 默认约束名称
	DefaultConstraintName string `json:"default_constraint_name,omitempty"`
}

// IndexColumn 索引所包含的单列定义
type IndexColumn struct {
	// Name 列名
	Name string `json:"name"`
	// SeqInIndex 列在索引中的序号（从1开始）
	SeqInIndex int `json:"seq_in_index"`
	// SubPart 索引前缀长度（如前10个字符），0表示全列索引
	SubPart *int `json:"sub_part"`
}

// IndexSchema 数据表索引元数据
type IndexSchema struct {
	// Name 索引名称（主键固定为 PRIMARY）
	Name string `json:"name"`
	// NonUnique 是否非唯一索引（false 表示 UNIQUE 唯一索引）
	NonUnique bool `json:"non_unique"`
	// IndexType 索引类型，如 BTREE, FULLTEXT, CLUSTERED, NONCLUSTERED
	IndexType string `json:"index_type"`
	// Columns 组成索引的列列表（按序号排序）
	Columns []IndexColumn `json:"columns"`
	// Comment 索引注释
	Comment string `json:"comment"`
	// Method PostgreSQL 索引方法，如 btree/gin/gist/brin
	Method string `json:"method,omitempty"`
	// IsClustered SQL Server 是否聚集索引
	IsClustered *bool `json:"is_clustered,omitempty"`
	// Filter 部分索引/筛选索引条件
	Filter string `json:"filter,omitempty"`
	// IncludeColumns 覆盖索引的 INCLUDE 列
	IncludeColumns []string `json:"include_columns,omitempty"`
	// IsConstraint 是否为约束型索引（UNIQUE 约束等）
	IsConstraint bool `json:"is_constraint,omitempty"`
	// ConstraintType 约束类型，如 UNIQUE
	ConstraintType string `json:"constraint_type,omitempty"`
}

// PrimaryKeySchema 表主键元数据
type PrimaryKeySchema struct {
	// Name 主键约束名称（MySQL 固定为 PRIMARY）
	Name string `json:"name,omitempty"`
	// Columns 主键包含的列名列表（按主键内顺序排列）
	Columns []string `json:"columns"`
	// IsClustered SQL Server 是否聚集主键
	IsClustered *bool `json:"is_clustered,omitempty"`
}

// TableSchema 单个数据表的完整元数据
type TableSchema struct {
	// Name 表名
	Name string `json:"name"`
	// Schema 表所属模式（MySQL 为空）
	Schema string `json:"schema,omitempty"`
	// Engine 存储引擎，如 InnoDB, MyISAM
	Engine string `json:"engine"`
	// Collation 表排序规则
	Collation string `json:"collation"`
	// Comment 表注释
	Comment string `json:"comment"`
	// CreateTableSQL 方言原生生成的建表语句（MySQL 为 SHOW CREATE TABLE 结果）
	CreateTableSQL string `json:"create_table_sql"`
	// Columns 表中所有字段清单（按字段位置排序）
	Columns []*ColumnSchema `json:"columns"`
	// Indexes 表中所有非主键索引清单
	Indexes []*IndexSchema `json:"indexes"`
	// PrimaryKey 表主键定义（若无主键则为 nil）
	PrimaryKey *PrimaryKeySchema `json:"primary_key"`
}

// GetColumn 根据列名查找字段元数据
func (t *TableSchema) GetColumn(name string) *ColumnSchema {
	for _, col := range t.Columns {
		if col.Name == name {
			return col
		}
	}
	return nil
}

// GetIndex 根据索引名查找索引元数据
func (t *TableSchema) GetIndex(name string) *IndexSchema {
	for _, idx := range t.Indexes {
		if idx.Name == name {
			return idx
		}
	}
	return nil
}

// DatabaseSchema 数据库层面的结构快照
type DatabaseSchema struct {
	// Vendor 数据库厂商
	Vendor Vendor `json:"vendor,omitempty"`
	// DatabaseName 数据库名
	DatabaseName string `json:"database_name"`
	// Schema 模式名（MySQL 为空，PostgreSQL/SQL Server 为 public/dbo 等）
	Schema string `json:"schema,omitempty"`
	// Tables 所有被监控表的结构映射表（key: 表名）
	Tables map[string]*TableSchema `json:"tables"`
}

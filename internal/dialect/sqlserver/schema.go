package sqlserver

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"sql-monitor/internal/dialect"
	"sql-monitor/internal/model"
)

// ExtractSchema 从 SQL Server 系统目录视图抓取目标模式下的表结构
func (p *Plugin) ExtractSchema(ctx context.Context, db *sql.DB, cfg *model.ConnectionConfig, f dialect.TableFilter) (*model.DatabaseSchema, error) {
	schemaName := cfg.EffectiveSchema()
	result := &model.DatabaseSchema{
		Vendor:       model.VendorSQLServer,
		DatabaseName: cfg.Database,
		Schema:       schemaName,
		Tables:       make(map[string]*model.TableSchema),
	}

	if err := p.extractTables(ctx, db, schemaName, f, result); err != nil {
		return nil, err
	}
	if len(result.Tables) == 0 {
		return result, nil
	}
	if err := p.extractColumns(ctx, db, schemaName, result); err != nil {
		return nil, err
	}
	if err := p.extractPrimaryKeys(ctx, db, schemaName, result); err != nil {
		return nil, err
	}
	if err := p.extractIndexes(ctx, db, schemaName, result); err != nil {
		return nil, err
	}
	return result, nil
}

// extractTables 抓取用户表及表注释
func (p *Plugin) extractTables(ctx context.Context, db *sql.DB, schemaName string, f dialect.TableFilter, result *model.DatabaseSchema) error {
	const query = `
		SELECT t.name, ISNULL(CAST(ep.value AS nvarchar(max)), N'')
		FROM sys.tables t
		INNER JOIN sys.schemas s ON t.schema_id = s.schema_id
		LEFT JOIN sys.extended_properties ep
			ON ep.class = 1 AND ep.major_id = t.object_id AND ep.minor_id = 0 AND ep.name = N'MS_Description'
		WHERE t.is_ms_shipped = 0 AND s.name = @p1
		ORDER BY t.name`

	rows, err := db.QueryContext(ctx, query, schemaName)
	if err != nil {
		return fmt.Errorf("查询数据表列表失败: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var name, comment string
		if err := rows.Scan(&name, &comment); err != nil {
			return fmt.Errorf("读取表元信息失败: %w", err)
		}
		if f != nil && !f.ShouldInclude(name) {
			continue
		}
		result.Tables[name] = &model.TableSchema{
			Name:       name,
			Schema:     schemaName,
			Comment:    comment,
			Columns:    make([]*model.ColumnSchema, 0),
			Indexes:    make([]*model.IndexSchema, 0),
			PrimaryKey: nil,
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("遍历数据表列表错误: %w", err)
	}
	return nil
}

// extractColumns 抓取字段类型、可空性、默认约束、IDENTITY、计算列与注释
func (p *Plugin) extractColumns(ctx context.Context, db *sql.DB, schemaName string, result *model.DatabaseSchema) error {
	const query = `
		SELECT
			t.name AS table_name,
			c.column_id,
			c.name AS column_name,
			ty.name AS type_name,
			c.max_length,
			c.precision,
			c.scale,
			c.is_nullable,
			c.is_identity,
			c.is_computed,
			cc.definition AS computed_definition,
			dc.name AS default_constraint_name,
			dc.definition AS default_definition,
			ISNULL(c.collation_name, N'') AS collation_name,
			ISNULL(CAST(ep.value AS nvarchar(max)), N'') AS column_comment
		FROM sys.columns c
		INNER JOIN sys.tables t ON c.object_id = t.object_id
		INNER JOIN sys.schemas s ON t.schema_id = s.schema_id
		INNER JOIN sys.types ty ON c.user_type_id = ty.user_type_id
		LEFT JOIN sys.computed_columns cc
			ON cc.object_id = c.object_id AND cc.column_id = c.column_id
		LEFT JOIN sys.default_constraints dc
			ON dc.parent_object_id = c.object_id AND dc.parent_column_id = c.column_id
		LEFT JOIN sys.extended_properties ep
			ON ep.class = 1 AND ep.major_id = c.object_id AND ep.minor_id = c.column_id AND ep.name = N'MS_Description'
		WHERE t.is_ms_shipped = 0 AND s.name = @p1
		ORDER BY t.name, c.column_id`

	rows, err := db.QueryContext(ctx, query, schemaName)
	if err != nil {
		return fmt.Errorf("查询数据表字段列表失败: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			tableName, columnName, typeName       string
			maxLength, precision, scale, columnID int
			isNullable, isIdentity, isComputed    bool
			computedDefinition                    sql.NullString
			defaultConstraintName, defaultDef     sql.NullString
			collationName, comment                string
		)
		if err := rows.Scan(
			&tableName, &columnID, &columnName, &typeName, &maxLength, &precision, &scale,
			&isNullable, &isIdentity, &isComputed, &computedDefinition,
			&defaultConstraintName, &defaultDef, &collationName, &comment,
		); err != nil {
			return fmt.Errorf("读取字段元信息失败: %w", err)
		}

		table, exists := result.Tables[tableName]
		if !exists {
			continue
		}

		typeName = strings.ToLower(typeName)
		col := &model.ColumnSchema{
			Name:            columnName,
			OrdinalPosition: columnID,
			ColumnType:      buildColumnType(typeName, maxLength, precision, scale),
			DataType:        typeName,
			IsNullable:      yesNo(isNullable),
			Comment:         comment,
			CollationName:   collationName,
			IsIdentity:      isIdentity,
			IsGenerated:     isComputed,
		}
		if isComputed && computedDefinition.Valid {
			col.GenerationExpression = computedDefinition.String
		}
		if defaultConstraintName.Valid {
			col.DefaultConstraintName = defaultConstraintName.String
		}
		if defaultDef.Valid {
			v := defaultDef.String
			col.ColumnDefault = &v
		}
		if isIdentity {
			col.Extra = "identity"
		}
		table.Columns = append(table.Columns, col)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("遍历字段列表错误: %w", err)
	}
	return nil
}

// buildColumnType 根据系统目录中的长度、精度与小数位还原完整列类型
func buildColumnType(typeName string, maxLength, precision, scale int) string {
	switch typeName {
	case "nvarchar", "nchar":
		if maxLength == -1 {
			return typeName + "(max)"
		}
		if maxLength > 0 {
			return fmt.Sprintf("%s(%d)", typeName, maxLength/2)
		}
	case "varchar", "char", "varbinary", "binary":
		if maxLength == -1 {
			return typeName + "(max)"
		}
		if maxLength > 0 {
			return fmt.Sprintf("%s(%d)", typeName, maxLength)
		}
	case "decimal", "numeric":
		return fmt.Sprintf("%s(%d,%d)", typeName, precision, scale)
	case "datetime2", "time", "datetimeoffset":
		return fmt.Sprintf("%s(%d)", typeName, scale)
	}
	return typeName
}

// yesNo 将布尔值转换为模型统一使用的 YES/NO 字符串
func yesNo(v bool) string {
	if v {
		return "YES"
	}
	return "NO"
}

// extractPrimaryKeys 抓取主键约束名称、列顺序与聚集方式
func (p *Plugin) extractPrimaryKeys(ctx context.Context, db *sql.DB, schemaName string, result *model.DatabaseSchema) error {
	const query = `
		SELECT t.name, kc.name, i.type_desc, c.name
		FROM sys.key_constraints kc
		INNER JOIN sys.tables t ON kc.parent_object_id = t.object_id
		INNER JOIN sys.schemas s ON t.schema_id = s.schema_id
		INNER JOIN sys.indexes i ON i.object_id = t.object_id AND i.index_id = kc.unique_index_id
		INNER JOIN sys.index_columns ic
			ON ic.object_id = i.object_id AND ic.index_id = i.index_id AND ic.key_ordinal > 0
		INNER JOIN sys.columns c ON c.object_id = ic.object_id AND c.column_id = ic.column_id
		WHERE kc.type = N'PK' AND t.is_ms_shipped = 0 AND s.name = @p1
		ORDER BY t.name, ic.key_ordinal`

	rows, err := db.QueryContext(ctx, query, schemaName)
	if err != nil {
		return fmt.Errorf("查询主键元数据失败: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var tableName, constraintName, typeDesc, columnName string
		if err := rows.Scan(&tableName, &constraintName, &typeDesc, &columnName); err != nil {
			return fmt.Errorf("读取主键记录失败: %w", err)
		}
		table, exists := result.Tables[tableName]
		if !exists {
			continue
		}
		if table.PrimaryKey == nil {
			clustered := strings.EqualFold(typeDesc, "CLUSTERED")
			table.PrimaryKey = &model.PrimaryKeySchema{
				Name:        constraintName,
				IsClustered: &clustered,
				Columns:     make([]string, 0),
			}
		}
		table.PrimaryKey.Columns = append(table.PrimaryKey.Columns, columnName)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("遍历主键列表错误: %w", err)
	}
	return nil
}

// indexBuilder 汇总同一索引的键列与包含列
type indexBuilder struct {
	name               string
	unique             bool
	typeDesc           string
	filter             string
	isUniqueConstraint bool
	constraintName     string
	keyColumns         []model.IndexColumn
	includeColumns     []string
}

// extractIndexes 抓取非主键索引、唯一约束、INCLUDE 列与筛选条件
func (p *Plugin) extractIndexes(ctx context.Context, db *sql.DB, schemaName string, result *model.DatabaseSchema) error {
	const query = `
		SELECT
			t.name AS table_name,
			i.name AS index_name,
			i.is_unique,
			i.type_desc,
			ISNULL(i.filter_definition, N'') AS filter_definition,
			i.is_unique_constraint,
			ISNULL((
				SELECT TOP (1) kc.name
				FROM sys.key_constraints kc
				WHERE kc.parent_object_id = i.object_id
				  AND kc.unique_index_id = i.index_id
				  AND kc.type = N'UQ'
			), N'') AS constraint_name,
			c.name AS column_name,
			ic.key_ordinal,
			ic.is_included_column,
			ic.index_ordinal
		FROM sys.indexes i
		INNER JOIN sys.tables t ON i.object_id = t.object_id
		INNER JOIN sys.schemas s ON t.schema_id = s.schema_id
		INNER JOIN sys.index_columns ic ON ic.object_id = i.object_id AND ic.index_id = i.index_id
		INNER JOIN sys.columns c ON c.object_id = ic.object_id AND c.column_id = ic.column_id
		WHERE i.index_id > 0
		  AND i.is_primary_key = 0
		  AND i.is_hypothetical = 0
		  AND t.is_ms_shipped = 0
		  AND s.name = @p1
		ORDER BY t.name, i.name, ic.is_included_column, ic.key_ordinal, ic.index_ordinal`

	rows, err := db.QueryContext(ctx, query, schemaName)
	if err != nil {
		return fmt.Errorf("查询索引元数据失败: %w", err)
	}
	defer rows.Close()

	tableIndexMap := make(map[string]map[string]*indexBuilder)
	for rows.Next() {
		var (
			tableName, indexName, typeDesc, filter, constraintName, columnName string
			unique, isUniqueConstraint, isIncluded                             bool
			keyOrdinal, indexOrdinal                                           int
		)
		if err := rows.Scan(
			&tableName, &indexName, &unique, &typeDesc, &filter, &isUniqueConstraint,
			&constraintName, &columnName, &keyOrdinal, &isIncluded, &indexOrdinal,
		); err != nil {
			return fmt.Errorf("读取索引记录失败: %w", err)
		}
		if _, exists := result.Tables[tableName]; !exists {
			continue
		}
		if _, ok := tableIndexMap[tableName]; !ok {
			tableIndexMap[tableName] = make(map[string]*indexBuilder)
		}
		builder, ok := tableIndexMap[tableName][indexName]
		if !ok {
			builder = &indexBuilder{
				name:               indexName,
				unique:             unique,
				typeDesc:           typeDesc,
				filter:             filter,
				isUniqueConstraint: isUniqueConstraint,
				constraintName:     constraintName,
			}
			tableIndexMap[tableName][indexName] = builder
		}
		if isIncluded {
			builder.includeColumns = append(builder.includeColumns, columnName)
		} else {
			builder.keyColumns = append(builder.keyColumns, model.IndexColumn{
				Name:       columnName,
				SeqInIndex: keyOrdinal,
			})
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("遍历索引列表错误: %w", err)
	}

	for tableName, builders := range tableIndexMap {
		table := result.Tables[tableName]
		for _, b := range builders {
			name := b.name
			// 唯一约束以约束名为准，便于后续使用 ALTER TABLE ... DROP CONSTRAINT
			if b.isUniqueConstraint && b.constraintName != "" {
				name = b.constraintName
			}
			clustered := strings.EqualFold(b.typeDesc, "CLUSTERED")
			idx := &model.IndexSchema{
				Name:           name,
				NonUnique:      !b.unique,
				IndexType:      b.typeDesc,
				Method:         b.typeDesc,
				Columns:        b.keyColumns,
				IsClustered:    &clustered,
				Filter:         b.filter,
				IncludeColumns: b.includeColumns,
			}
			if b.isUniqueConstraint {
				idx.IsConstraint = true
				idx.ConstraintType = "UNIQUE"
			}
			table.Indexes = append(table.Indexes, idx)
		}
	}
	return nil
}

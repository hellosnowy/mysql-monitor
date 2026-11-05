package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"sql-monitor/internal/dialect"
	"sql-monitor/internal/model"
)

// ExtractSchema 从 pg_catalog 抓取目标模式下符合过滤条件的表结构
func (p *Plugin) ExtractSchema(ctx context.Context, db *sql.DB, cfg *model.ConnectionConfig, f dialect.TableFilter) (*model.DatabaseSchema, error) {
	schemaName := cfg.EffectiveSchema()
	schema := &model.DatabaseSchema{
		Vendor:       model.VendorPostgres,
		DatabaseName: cfg.Database,
		Schema:       schemaName,
		Tables:       make(map[string]*model.TableSchema),
	}

	const tablesQuery = `
		SELECT c.relname,
		       COALESCE(pg_catalog.obj_description(c.oid, 'pg_class'), '')
		FROM pg_catalog.pg_class c
		JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = $1
		  AND c.relkind IN ('r', 'p')
		ORDER BY c.relname`

	rows, err := db.QueryContext(ctx, tablesQuery, schemaName)
	if err != nil {
		return nil, fmt.Errorf("查询数据表列表失败: %w", err)
	}

	for rows.Next() {
		var name, comment string
		if err := rows.Scan(&name, &comment); err != nil {
			rows.Close()
			return nil, fmt.Errorf("读取表元信息失败: %w", err)
		}
		if f != nil && !f.ShouldInclude(name) {
			continue
		}
		schema.Tables[name] = &model.TableSchema{
			Name:       name,
			Schema:     schemaName,
			Comment:    comment,
			Columns:    make([]*model.ColumnSchema, 0),
			Indexes:    make([]*model.IndexSchema, 0),
			PrimaryKey: nil,
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("遍历数据表列表错误: %w", err)
	}
	rows.Close()

	if len(schema.Tables) == 0 {
		return schema, nil
	}

	if err := p.extractColumns(ctx, db, schemaName, schema); err != nil {
		return nil, err
	}
	if err := p.extractPrimaryKeys(ctx, db, schemaName, schema); err != nil {
		return nil, err
	}
	if err := p.extractIndexes(ctx, db, schemaName, schema); err != nil {
		return nil, err
	}

	return schema, nil
}

// extractColumns 抓取字段类型、默认值、自增/生成列、注释与排序规则
func (p *Plugin) extractColumns(ctx context.Context, db *sql.DB, schemaName string, schema *model.DatabaseSchema) error {
	const columnsQuery = `
		SELECT
			c.relname,
			a.attname,
			a.attnum,
			pg_catalog.format_type(a.atttypid, a.atttypmod),
			t.typname,
			a.attnotnull,
			COALESCE(pg_catalog.pg_get_expr(ad.adbin, ad.adrelid), ''),
			a.attidentity::text,
			a.attgenerated::text,
			COALESCE(pg_catalog.col_description(c.oid, a.attnum), ''),
			COALESCE(coll.collname, '')
		FROM pg_catalog.pg_attribute a
		JOIN pg_catalog.pg_class c ON c.oid = a.attrelid
		JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
		JOIN pg_catalog.pg_type t ON t.oid = a.atttypid
		LEFT JOIN pg_catalog.pg_attrdef ad
		       ON ad.adrelid = a.attrelid AND ad.adnum = a.attnum
		LEFT JOIN pg_catalog.pg_collation coll ON coll.oid = a.attcollation
		WHERE n.nspname = $1
		  AND c.relkind IN ('r', 'p')
		  AND a.attnum > 0
		  AND NOT a.attisdropped
		ORDER BY c.relname, a.attnum`

	rows, err := db.QueryContext(ctx, columnsQuery, schemaName)
	if err != nil {
		return fmt.Errorf("查询数据表字段列表失败: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			tableName    string
			colName      string
			ordinalPos   int
			columnType   string
			dataType     string
			notNull      bool
			defaultExpr  string
			attIdentity  string
			attGenerated string
			comment      string
			collation    string
		)
		if err := rows.Scan(&tableName, &colName, &ordinalPos, &columnType, &dataType,
			&notNull, &defaultExpr, &attIdentity, &attGenerated, &comment, &collation); err != nil {
			return fmt.Errorf("读取字段元信息失败: %w", err)
		}

		table, exists := schema.Tables[tableName]
		if !exists {
			continue
		}

		nullable := "YES"
		if notNull {
			nullable = "NO"
		}
		defaultExpr = strings.TrimSpace(defaultExpr)

		col := &model.ColumnSchema{
			Name:            colName,
			OrdinalPosition: ordinalPos,
			ColumnType:      columnType,
			DataType:        dataType,
			IsNullable:      nullable,
			Comment:         comment,
			CollationName:   collation,
		}

		switch {
		case attIdentity != "":
			// PostgreSQL identity 列：ALWAYS 或 BY DEFAULT
			col.IsIdentity = true
			if attIdentity == "a" {
				col.IdentityGeneration = "ALWAYS"
			} else {
				col.IdentityGeneration = "BY DEFAULT"
			}
		case attGenerated != "":
			// 生成列（当前抓取 STORED 生成列），表达式来自 pg_attrdef
			col.IsGenerated = true
			col.GenerationExpression = defaultExpr
		case defaultExpr != "":
			// serial 本质是整数列 + nextval 默认值，改用 serial/bigserial 表示，
			// 避免建表语句引用目标库中不存在的序列。
			if serialType := serialColumnType(dataType, columnType, defaultExpr); serialType != "" {
				col.ColumnType = serialType
			} else {
				def := defaultExpr
				col.ColumnDefault = &def
			}
		}

		table.Columns = append(table.Columns, col)
	}
	return rows.Err()
}

// serialColumnType 判断默认值是否为 nextval 序列调用，并返回对应的 serial 类型
func serialColumnType(dataType, columnType, defaultExpr string) string {
	if !strings.HasPrefix(strings.ToLower(defaultExpr), "nextval(") {
		return ""
	}
	switch strings.ToLower(dataType) {
	case "int4":
		return "serial"
	case "int8":
		return "bigserial"
	}
	switch strings.ToLower(columnType) {
	case "integer":
		return "serial"
	case "bigint":
		return "bigserial"
	}
	return ""
}

// extractPrimaryKeys 抓取主键约束名称与按约束顺序排列的列
func (p *Plugin) extractPrimaryKeys(ctx context.Context, db *sql.DB, schemaName string, schema *model.DatabaseSchema) error {
	const pkQuery = `
		SELECT c.relname, con.conname, a.attname
		FROM pg_catalog.pg_constraint con
		JOIN pg_catalog.pg_class c ON c.oid = con.conrelid
		JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
		JOIN LATERAL unnest(con.conkey) WITH ORDINALITY AS ord(attnum, ordinality) ON true
		JOIN pg_catalog.pg_attribute a ON a.attrelid = con.conrelid AND a.attnum = ord.attnum
		WHERE n.nspname = $1
		  AND con.contype = 'p'
		  AND c.relkind IN ('r', 'p')
		ORDER BY c.relname, ord.ordinality`

	rows, err := db.QueryContext(ctx, pkQuery, schemaName)
	if err != nil {
		return fmt.Errorf("查询主键元数据失败: %w", err)
	}
	defer rows.Close()

	pkMap := make(map[string]*model.PrimaryKeySchema)
	for rows.Next() {
		var tableName, constraintName, colName string
		if err := rows.Scan(&tableName, &constraintName, &colName); err != nil {
			return fmt.Errorf("读取主键记录失败: %w", err)
		}
		if _, exists := schema.Tables[tableName]; !exists {
			continue
		}
		pk, ok := pkMap[tableName]
		if !ok {
			pk = &model.PrimaryKeySchema{Name: constraintName}
			pkMap[tableName] = pk
		}
		pk.Columns = append(pk.Columns, colName)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for tableName, pk := range pkMap {
		if table, exists := schema.Tables[tableName]; exists {
			table.PrimaryKey = pk
		}
	}
	return nil
}

// pgIndexBuilder 汇总同一索引的多列记录
type pgIndexBuilder struct {
	name           string
	nonUnique      bool
	method         string
	filter         string
	constraintType string
	columns        []model.IndexColumn
	includeColumns []string
}

// extractIndexes 抓取普通索引、唯一索引、部分索引与约束型索引
func (p *Plugin) extractIndexes(ctx context.Context, db *sql.DB, schemaName string, schema *model.DatabaseSchema) error {
	const indexQuery = `
		SELECT
			c.relname,
			ic.relname,
			i.indisunique,
			am.amname,
			COALESCE(pg_catalog.pg_get_expr(i.indpred, i.indrelid), ''),
			COALESCE(con.contype::text, ''),
			COALESCE(con.conname, ''),
			COALESCE(a.attname, pg_catalog.pg_get_indexdef(i.indexrelid, ord.ordinality::int, true)),
			ord.ordinality,
			i.indnkeyatts
		FROM pg_catalog.pg_index i
		JOIN pg_catalog.pg_class c ON c.oid = i.indrelid
		JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
		JOIN pg_catalog.pg_class ic ON ic.oid = i.indexrelid
		JOIN pg_catalog.pg_am am ON am.oid = ic.relam
		LEFT JOIN pg_catalog.pg_constraint con
		       ON con.conindid = i.indexrelid AND con.conrelid = i.indrelid
		JOIN LATERAL unnest(i.indkey) WITH ORDINALITY AS ord(attnum, ordinality) ON true
		LEFT JOIN pg_catalog.pg_attribute a
		       ON a.attrelid = i.indrelid AND a.attnum = ord.attnum
		WHERE n.nspname = $1
		  AND c.relkind IN ('r', 'p')
		  AND NOT i.indisprimary
		ORDER BY c.relname, ic.relname, ord.ordinality`

	rows, err := db.QueryContext(ctx, indexQuery, schemaName)
	if err != nil {
		return fmt.Errorf("查询索引元数据失败: %w", err)
	}
	defer rows.Close()

	tableIndexMap := make(map[string]map[string]*pgIndexBuilder)
	for rows.Next() {
		var (
			tableName      string
			indexName      string
			unique         bool
			method         string
			filter         string
			constraintType string
			constraintName string
			columnName     string
			seq            int
			keyCount       int
		)
		if err := rows.Scan(&tableName, &indexName, &unique, &method, &filter,
			&constraintType, &constraintName, &columnName, &seq, &keyCount); err != nil {
			return fmt.Errorf("读取索引记录失败: %w", err)
		}
		if _, exists := schema.Tables[tableName]; !exists {
			continue
		}
		if constraintType == "p" {
			continue
		}

		if _, ok := tableIndexMap[tableName]; !ok {
			tableIndexMap[tableName] = make(map[string]*pgIndexBuilder)
		}
		builder, ok := tableIndexMap[tableName][indexName]
		if !ok {
			name := indexName
			if constraintType != "" && constraintName != "" {
				// 约束型索引在增删语句中必须使用约束名称
				name = constraintName
			}
			builder = &pgIndexBuilder{
				name:           name,
				nonUnique:      !unique,
				method:         method,
				filter:         filter,
				constraintType: constraintType,
			}
			tableIndexMap[tableName][indexName] = builder
		}

		if seq <= keyCount {
			builder.columns = append(builder.columns, model.IndexColumn{
				Name:       columnName,
				SeqInIndex: seq,
			})
		} else {
			// indnkeyatts 之后为 INCLUDE 覆盖列
			builder.includeColumns = append(builder.includeColumns, columnName)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for tableName, indexMap := range tableIndexMap {
		table, exists := schema.Tables[tableName]
		if !exists {
			continue
		}
		names := make([]string, 0, len(indexMap))
		for name := range indexMap {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			b := indexMap[name]
			table.Indexes = append(table.Indexes, &model.IndexSchema{
				Name:           b.name,
				NonUnique:      b.nonUnique,
				IndexType:      b.method,
				Method:         b.method,
				Columns:        b.columns,
				Filter:         b.filter,
				IncludeColumns: b.includeColumns,
				IsConstraint:   b.constraintType != "",
				ConstraintType: constraintTypeName(b.constraintType),
			})
		}
	}
	return nil
}

// constraintTypeName 将 pg_constraint.contype 转换为可读约束类型
func constraintTypeName(contype string) string {
	switch contype {
	case "u":
		return "UNIQUE"
	case "x":
		return "EXCLUDE"
	case "p":
		return "PRIMARY"
	case "":
		return ""
	default:
		return strings.ToUpper(contype)
	}
}

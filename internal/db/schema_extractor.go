package db

import (
	"context"
	"database/sql"
	"fmt"

	"mysql-monitor/internal/filter"
	"mysql-monitor/internal/model"
	"mysql-monitor/pkg/sqlutil"
)

// SchemaExtractor MySQL 表结构提取器
type SchemaExtractor struct {
	db     *sql.DB
	dbName string
	filter *filter.TableFilter
}

// NewSchemaExtractor 创建表结构提取器
func NewSchemaExtractor(db *sql.DB, dbName string, tableFilter *filter.TableFilter) *SchemaExtractor {
	return &SchemaExtractor{
		db:     db,
		dbName: dbName,
		filter: tableFilter,
	}
}

// ExtractDatabaseSchema 从 MySQL 抓取所有符合黑白名单过滤条件的表结构
func (e *SchemaExtractor) ExtractDatabaseSchema(ctx context.Context) (*model.DatabaseSchema, error) {
	// 1. 查询目标库下的所有物理表
	tablesQuery := `
		SELECT TABLE_NAME, IFNULL(ENGINE, ''), IFNULL(TABLE_COLLATION, ''), IFNULL(TABLE_COMMENT, '')
		FROM information_schema.TABLES
		WHERE TABLE_SCHEMA = ? AND TABLE_TYPE = 'BASE TABLE'
		ORDER BY TABLE_NAME`
	rows, err := e.db.QueryContext(ctx, tablesQuery, e.dbName)
	if err != nil {
		return nil, fmt.Errorf("查询数据表列表失败: %w", err)
	}
	defer rows.Close()

	tables := make(map[string]*model.TableSchema)
	var filteredTableNames []string

	for rows.Next() {
		var name, engine, collation, comment string
		if err := rows.Scan(&name, &engine, &collation, &comment); err != nil {
			return nil, fmt.Errorf("读取表元信息失败: %w", err)
		}

		// 执行黑白名单过滤
		if e.filter != nil && !e.filter.ShouldInclude(name) {
			continue
		}

		tableSchema := &model.TableSchema{
			Name:       name,
			Engine:     engine,
			Collation:  collation,
			Comment:    comment,
			Columns:    make([]*model.ColumnSchema, 0),
			Indexes:    make([]*model.IndexSchema, 0),
			PrimaryKey: nil,
		}
		tables[name] = tableSchema
		filteredTableNames = append(filteredTableNames, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历数据表列表错误: %w", err)
	}

	if len(tables) == 0 {
		return &model.DatabaseSchema{
			DatabaseName: e.dbName,
			Tables:       tables,
		}, nil
	}

	// 2. 提取各表的字段信息
	if err := e.extractColumns(ctx, tables); err != nil {
		return nil, err
	}

	// 3. 提取各表的主键与非主键索引
	if err := e.extractIndexes(ctx, tables); err != nil {
		return nil, err
	}

	// 4. 提取各表的原始 SHOW CREATE TABLE 语句
	for _, tableName := range filteredTableNames {
		createSQL, err := e.getShowCreateTable(ctx, tableName)
		if err == nil {
			tables[tableName].CreateTableSQL = createSQL
		}
	}

	return &model.DatabaseSchema{
		DatabaseName: e.dbName,
		Tables:       tables,
	}, nil
}

// extractColumns 从 information_schema.COLUMNS 批量提取列信息
func (e *SchemaExtractor) extractColumns(ctx context.Context, tables map[string]*model.TableSchema) error {
	colQuery := `
		SELECT 
			TABLE_NAME, 
			COLUMN_NAME, 
			ORDINAL_POSITION, 
			COLUMN_DEFAULT, 
			IS_NULLABLE, 
			DATA_TYPE, 
			COLUMN_TYPE, 
			EXTRA, 
			COLUMN_COMMENT, 
			IFNULL(CHARACTER_SET_NAME, ''), 
			IFNULL(COLLATION_NAME, '')
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = ?
		ORDER BY TABLE_NAME, ORDINAL_POSITION`

	rows, err := e.db.QueryContext(ctx, colQuery, e.dbName)
	if err != nil {
		return fmt.Errorf("查询数据表字段列表失败: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var tableName, colName, isNullable, dataType, colType, extra, comment, charSet, collation string
		var ordPos int
		var defaultVal sql.NullString

		if err := rows.Scan(&tableName, &colName, &ordPos, &defaultVal, &isNullable, &dataType, &colType, &extra, &comment, &charSet, &collation); err != nil {
			return fmt.Errorf("读取字段元信息失败: %w", err)
		}

		table, exists := tables[tableName]
		if !exists {
			continue
		}

		var defPtr *string
		if defaultVal.Valid {
			v := defaultVal.String
			defPtr = &v
		}

		col := &model.ColumnSchema{
			Name:             colName,
			OrdinalPosition:  ordPos,
			ColumnType:       colType,
			DataType:         dataType,
			IsNullable:       isNullable,
			ColumnDefault:    defPtr,
			Extra:            extra,
			Comment:          comment,
			CharacterSetName: charSet,
			CollationName:    collation,
		}
		table.Columns = append(table.Columns, col)
	}

	return rows.Err()
}

// extractIndexes 从 information_schema.STATISTICS 提取主键和普通/唯一索引
func (e *SchemaExtractor) extractIndexes(ctx context.Context, tables map[string]*model.TableSchema) error {
	idxQuery := `
		SELECT 
			TABLE_NAME, 
			NON_UNIQUE, 
			INDEX_NAME, 
			SEQ_IN_INDEX, 
			COLUMN_NAME, 
			SUB_PART, 
			INDEX_TYPE, 
			INDEX_COMMENT
		FROM information_schema.STATISTICS
		WHERE TABLE_SCHEMA = ?
		ORDER BY TABLE_NAME, INDEX_NAME, SEQ_IN_INDEX`

	rows, err := e.db.QueryContext(ctx, idxQuery, e.dbName)
	if err != nil {
		return fmt.Errorf("查询索引元数据失败: %w", err)
	}
	defer rows.Close()

	// 缓存每个表正在组装的索引
	type indexBuilder struct {
		name      string
		nonUnique bool
		indexType string
		comment   string
		columns   []model.IndexColumn
	}
	tableIndexMap := make(map[string]map[string]*indexBuilder)
	tablePKMap := make(map[string][]string)

	for rows.Next() {
		var tableName, idxName, colName, idxType, comment string
		var nonUnique int
		var seqInIndex int
		var subPart sql.NullInt64

		if err := rows.Scan(&tableName, &nonUnique, &idxName, &seqInIndex, &colName, &subPart, &idxType, &comment); err != nil {
			return fmt.Errorf("读取索引记录失败: %w", err)
		}

		if _, exists := tables[tableName]; !exists {
			continue
		}

		// 处理主键 PRIMARY
		if idxName == "PRIMARY" {
			tablePKMap[tableName] = append(tablePKMap[tableName], colName)
			continue
		}

		// 处理非主键索引
		if _, ok := tableIndexMap[tableName]; !ok {
			tableIndexMap[tableName] = make(map[string]*indexBuilder)
		}

		builder, ok := tableIndexMap[tableName][idxName]
		if !ok {
			builder = &indexBuilder{
				name:      idxName,
				nonUnique: nonUnique == 1,
				indexType: idxType,
				comment:   comment,
			}
			tableIndexMap[tableName][idxName] = builder
		}

		var subPartPtr *int
		if subPart.Valid {
			val := int(subPart.Int64)
			subPartPtr = &val
		}

		builder.columns = append(builder.columns, model.IndexColumn{
			Name:       colName,
			SeqInIndex: seqInIndex,
			SubPart:    subPartPtr,
		})
	}

	// 赋值回各个表对象
	for tableName, table := range tables {
		if pkCols, ok := tablePKMap[tableName]; ok && len(pkCols) > 0 {
			table.PrimaryKey = &model.PrimaryKeySchema{
				Columns: pkCols,
			}
		}

		if idxMap, ok := tableIndexMap[tableName]; ok {
			for _, b := range idxMap {
				table.Indexes = append(table.Indexes, &model.IndexSchema{
					Name:      b.name,
					NonUnique: b.nonUnique,
					IndexType: b.indexType,
					Columns:   b.columns,
					Comment:   b.comment,
				})
			}
		}
	}

	return rows.Err()
}

// getShowCreateTable 执行 SHOW CREATE TABLE 抓取原生建表语句
func (e *SchemaExtractor) getShowCreateTable(ctx context.Context, tableName string) (string, error) {
	query := fmt.Sprintf("SHOW CREATE TABLE %s", sqlutil.EscapeIdentifier(tableName))
	var tblName, createSQL string
	err := e.db.QueryRowContext(ctx, query).Scan(&tblName, &createSQL)
	if err != nil {
		return "", err
	}
	return createSQL, nil
}

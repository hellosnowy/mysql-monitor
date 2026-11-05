package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"sql-monitor/internal/dialect"
	"sql-monitor/internal/model"
	"sql-monitor/pkg/sqlutil"
)

// ExtractSchema 从 MySQL 抓取目标库下符合过滤条件的表结构
func (p *Plugin) ExtractSchema(ctx context.Context, db *sql.DB, cfg *model.ConnectionConfig, f dialect.TableFilter) (*model.DatabaseSchema, error) {
	dbName := cfg.Database
	schema := &model.DatabaseSchema{
		Vendor:       model.VendorMySQL,
		DatabaseName: dbName,
		Tables:       make(map[string]*model.TableSchema),
	}

	tablesQuery := `
		SELECT TABLE_NAME, IFNULL(ENGINE, ''), IFNULL(TABLE_COLLATION, ''), IFNULL(TABLE_COMMENT, '')
		FROM information_schema.TABLES
		WHERE TABLE_SCHEMA = ? AND TABLE_TYPE = 'BASE TABLE'
		ORDER BY TABLE_NAME`
	rows, err := db.QueryContext(ctx, tablesQuery, dbName)
	if err != nil {
		return nil, fmt.Errorf("查询数据表列表失败: %w", err)
	}

	var filteredTableNames []string
	for rows.Next() {
		var name, engine, collation, comment string
		if err := rows.Scan(&name, &engine, &collation, &comment); err != nil {
			rows.Close()
			return nil, fmt.Errorf("读取表元信息失败: %w", err)
		}
		if f != nil && !f.ShouldInclude(name) {
			continue
		}
		schema.Tables[name] = &model.TableSchema{
			Name:       name,
			Engine:     engine,
			Collation:  collation,
			Comment:    comment,
			Columns:    make([]*model.ColumnSchema, 0),
			Indexes:    make([]*model.IndexSchema, 0),
			PrimaryKey: nil,
		}
		filteredTableNames = append(filteredTableNames, name)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("遍历数据表列表错误: %w", err)
	}
	rows.Close()

	if len(schema.Tables) == 0 {
		return schema, nil
	}

	if err := p.extractColumns(ctx, db, dbName, schema); err != nil {
		return nil, err
	}
	if err := p.extractIndexes(ctx, db, dbName, schema); err != nil {
		return nil, err
	}
	for _, tableName := range filteredTableNames {
		createSQL, err := getShowCreateTable(ctx, db, tableName)
		if err == nil {
			schema.Tables[tableName].CreateTableSQL = createSQL
		}
	}

	return schema, nil
}

func (p *Plugin) extractColumns(ctx context.Context, db *sql.DB, dbName string, schema *model.DatabaseSchema) error {
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

	rows, err := db.QueryContext(ctx, colQuery, dbName)
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
		table, exists := schema.Tables[tableName]
		if !exists {
			continue
		}

		var defPtr *string
		if defaultVal.Valid {
			v := defaultVal.String
			defPtr = &v
		}

		table.Columns = append(table.Columns, &model.ColumnSchema{
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
			IsIdentity:       containsFold(extra, "auto_increment"),
		})
	}
	return rows.Err()
}

func (p *Plugin) extractIndexes(ctx context.Context, db *sql.DB, dbName string, schema *model.DatabaseSchema) error {
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

	rows, err := db.QueryContext(ctx, idxQuery, dbName)
	if err != nil {
		return fmt.Errorf("查询索引元数据失败: %w", err)
	}
	defer rows.Close()

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
		if _, exists := schema.Tables[tableName]; !exists {
			continue
		}

		if idxName == "PRIMARY" {
			tablePKMap[tableName] = append(tablePKMap[tableName], colName)
			continue
		}

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
			v := int(subPart.Int64)
			subPartPtr = &v
		}
		builder.columns = append(builder.columns, model.IndexColumn{
			Name:       colName,
			SeqInIndex: seqInIndex,
			SubPart:    subPartPtr,
		})
	}

	for tableName, table := range schema.Tables {
		if pkCols, ok := tablePKMap[tableName]; ok && len(pkCols) > 0 {
			table.PrimaryKey = &model.PrimaryKeySchema{Name: "PRIMARY", Columns: pkCols}
		}
		if idxMap, ok := tableIndexMap[tableName]; ok {
			for _, b := range idxMap {
				table.Indexes = append(table.Indexes, &model.IndexSchema{
					Name:      b.name,
					NonUnique: b.nonUnique,
					IndexType: b.indexType,
					Method:    b.indexType,
					Columns:   b.columns,
					Comment:   b.comment,
				})
			}
		}
	}
	return rows.Err()
}

func getShowCreateTable(ctx context.Context, db *sql.DB, tableName string) (string, error) {
	query := fmt.Sprintf("SHOW CREATE TABLE %s", sqlutil.EscapeIdentifier(tableName))
	var tblName, createSQL string
	if err := db.QueryRowContext(ctx, query).Scan(&tblName, &createSQL); err != nil {
		return "", err
	}
	return createSQL, nil
}

func containsFold(s, sub string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}

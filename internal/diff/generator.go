package diff

import (
	"fmt"
	"strings"
	"time"

	"sql-monitor/internal/model"
	"sql-monitor/internal/sqlgen"
)

// GenerateDiffResult 综合执行 DDL 与 DML 比对，生成包含统计概览与可执行 SQL 脚本的完整比对结果。
// direction 为 rollback 时内部交换两个快照，生成“由目标版本回滚到起始版本”的 SQL。
func GenerateDiffResult(connID, fromVer, toVer string, fromSnap, toSnap *model.Snapshot, direction model.DiffDirection, em sqlgen.Emitter) *model.DiffResult {
	direction = model.NormalizeDirection(direction)

	effectiveFrom, effectiveTo := fromSnap, toSnap
	if direction == model.DirectionRollback {
		effectiveFrom, effectiveTo = toSnap, fromSnap
	}

	var fromSchema, toSchema *model.DatabaseSchema
	var fromData, toData *model.DatabaseData
	if effectiveFrom != nil {
		fromSchema = effectiveFrom.Schema
		fromData = effectiveFrom.Data
	}
	if effectiveTo != nil {
		toSchema = effectiveTo.Schema
		toData = effectiveTo.Data
	}

	// 1. 运行 DDL 差异比对
	tableDDLs, ddlSummary := DiffSchemas(fromSchema, toSchema, em)

	// 2. 运行 DML 差异比对。结构快照没有行数据，不能解释为全表清空。
	var tableDMLs []*model.TableDMLDiff
	var dmlSummary model.DMLDiffSummary
	if !isSchemaOnlySnapshot(effectiveFrom) && !isSchemaOnlySnapshot(effectiveTo) {
		// 整表删除由 DDL 处理，避免在 DROP TABLE 之后再生成 DELETE。
		if fromData != nil && toSchema != nil {
			filtered := &model.DatabaseData{Tables: make(map[string]*model.TableData)}
			for name, table := range fromData.Tables {
				if _, exists := toSchema.Tables[name]; exists {
					filtered.Tables[name] = table
				}
			}
			fromData = filtered
		}
		tableDMLs, dmlSummary = DiffData(fromData, toData, toSchema, em)
	}

	// 3. 构造纯 DDL 脚本
	var ddlScriptBuilder strings.Builder
	for _, td := range tableDDLs {
		ddlScriptBuilder.WriteString(fmt.Sprintf("-- 表: %s (%s)\n", td.TableName, td.DiffType))
		for _, d := range td.Details {
			ddlScriptBuilder.WriteString(fmt.Sprintf("--   * %s\n", d))
		}
		for _, stmt := range td.Statements {
			ddlScriptBuilder.WriteString(stmt)
			ddlScriptBuilder.WriteString("\n")
		}
		ddlScriptBuilder.WriteString("\n")
	}

	// 4. 构造纯 DML 脚本
	var dmlScriptBuilder strings.Builder
	for _, td := range tableDMLs {
		dmlScriptBuilder.WriteString(fmt.Sprintf("-- 表: %s (新增: %d, 更新: %d, 删除: %d)\n",
			td.TableName, td.InsertCount, td.UpdateCount, td.DeleteCount))

		tableSchema := lookupTableSchema(toSchema, td.TableName)
		hasInserts := td.InsertCount > 0
		for _, stmt := range em.TableDMLBegin(tableSchema, hasInserts) {
			dmlScriptBuilder.WriteString(stmt)
			dmlScriptBuilder.WriteString("\n")
		}
		for _, stmt := range td.DeleteStatements {
			dmlScriptBuilder.WriteString(stmt)
			dmlScriptBuilder.WriteString("\n")
		}
		for _, stmt := range td.UpdateStatements {
			dmlScriptBuilder.WriteString(stmt)
			dmlScriptBuilder.WriteString("\n")
		}
		for _, stmt := range td.InsertStatements {
			dmlScriptBuilder.WriteString(stmt)
			dmlScriptBuilder.WriteString("\n")
		}
		for _, stmt := range em.TableDMLEnd(tableSchema, hasInserts) {
			dmlScriptBuilder.WriteString(stmt)
			dmlScriptBuilder.WriteString("\n")
		}
		dmlScriptBuilder.WriteString("\n")
	}

	now := time.Now()
	res := &model.DiffResult{
		ConnectionID:  connID,
		Vendor:        em.Vendor(),
		Direction:     direction,
		FromVersionID: fromVer,
		ToVersionID:   toVer,
		ComparedAt:    now,
		Summary: model.DiffSummary{
			DDL: ddlSummary,
			DML: dmlSummary,
		},
		TableDDLDiffs: tableDDLs,
		TableDMLDiffs: tableDMLs,
		DDLScript:     strings.TrimSpace(ddlScriptBuilder.String()),
		DMLScript:     strings.TrimSpace(dmlScriptBuilder.String()),
	}

	// 5. 构造带事务安全包裹的完整脚本
	var fullBuilder strings.Builder
	fullBuilder.WriteString("-- ====================================================================\n")
	fullBuilder.WriteString(fmt.Sprintf("-- SQL Monitor 版本差异同步脚本 (%s)\n", em.Vendor().DisplayName()))
	fullBuilder.WriteString(fmt.Sprintf("-- 连接标识: %s\n", connID))
	if direction == model.DirectionRollback {
		fullBuilder.WriteString(fmt.Sprintf("-- 方向: 回滚 (%s -> %s)\n", toVer, fromVer))
	} else {
		fullBuilder.WriteString(fmt.Sprintf("-- 方向: 升级 (%s -> %s)\n", fromVer, toVer))
	}
	fullBuilder.WriteString(fmt.Sprintf("-- 选择版本: 起始=%s 目标=%s\n", fromVer, toVer))
	fullBuilder.WriteString(fmt.Sprintf("-- 生成时间: %s\n", now.Format("2006-01-02 15:04:05")))
	fullBuilder.WriteString("--\n")
	fullBuilder.WriteString(fmt.Sprintf("-- DDL 变动: 新建表 %d, 删除表 %d, 变更表 %d | 新增字段 %d, 删除字段 %d, 修改字段 %d | 新增索引 %d, 删除索引 %d\n",
		ddlSummary.AddedTables, ddlSummary.DroppedTables, ddlSummary.ModifiedTables,
		ddlSummary.AddedColumns, ddlSummary.DroppedColumns, ddlSummary.ModifiedColumns,
		ddlSummary.AddedIndexes, ddlSummary.DroppedIndexes))
	fullBuilder.WriteString(fmt.Sprintf("-- DML 变动: 插入数据 %d 行, 修改数据 %d 行, 删除数据 %d 行\n",
		dmlSummary.InsertCount, dmlSummary.UpdateCount, dmlSummary.DeleteCount))
	fullBuilder.WriteString("-- ====================================================================\n\n")

	sessionBegin := em.SessionBegin()
	for _, stmt := range sessionBegin {
		fullBuilder.WriteString(stmt)
		fullBuilder.WriteString("\n")
	}
	if len(sessionBegin) > 0 {
		fullBuilder.WriteString("\n")
	}

	if res.DDLScript != "" {
		fullBuilder.WriteString("-- --------------------------------------------------------------------\n")
		fullBuilder.WriteString("-- 1. DDL 结构变更\n")
		fullBuilder.WriteString("-- --------------------------------------------------------------------\n")
		for _, stmt := range em.DDLBegin() {
			fullBuilder.WriteString(stmt)
			fullBuilder.WriteString("\n")
		}
		fullBuilder.WriteString(res.DDLScript)
		fullBuilder.WriteString("\n")
		for _, stmt := range em.DDLEnd() {
			fullBuilder.WriteString(stmt)
			fullBuilder.WriteString("\n")
		}
		fullBuilder.WriteString("\n")
	}

	if res.DMLScript != "" {
		fullBuilder.WriteString("-- --------------------------------------------------------------------\n")
		fullBuilder.WriteString("-- 2. DML 数据变更\n")
		fullBuilder.WriteString("-- --------------------------------------------------------------------\n")
		for _, stmt := range em.DMLBegin() {
			fullBuilder.WriteString(stmt)
			fullBuilder.WriteString("\n")
		}
		fullBuilder.WriteString(res.DMLScript)
		fullBuilder.WriteString("\n")
		for _, stmt := range em.DMLEnd() {
			fullBuilder.WriteString(stmt)
			fullBuilder.WriteString("\n")
		}
		fullBuilder.WriteString("\n")
	}

	for _, stmt := range em.SessionEnd() {
		fullBuilder.WriteString(stmt)
		fullBuilder.WriteString("\n")
	}
	res.FullScript = strings.TrimSpace(fullBuilder.String())

	return res
}

// isSchemaOnlySnapshot 判断快照是否仅保存结构而没有采集行数据。
func isSchemaOnlySnapshot(snapshot *model.Snapshot) bool {
	return snapshot != nil && snapshot.Schema != nil && len(snapshot.Schema.Tables) > 0 &&
		(snapshot.Data == nil || len(snapshot.Data.Tables) == 0)
}

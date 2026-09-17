package diff

import (
	"fmt"
	"strings"
	"time"

	"mysql-monitor/internal/model"
)

// GenerateDiffResult 综合执行 DDL 与 DML 比对，生成包含统计概览与可执行 SQL 脚本的完整比对结果
func GenerateDiffResult(connID, fromVer, toVer string, fromSnap, toSnap *model.Snapshot) *model.DiffResult {
	var fromSchema *model.DatabaseSchema
	var toSchema *model.DatabaseSchema
	var fromData *model.DatabaseData
	var toData *model.DatabaseData

	if fromSnap != nil {
		fromSchema = fromSnap.Schema
		fromData = fromSnap.Data
	}
	if toSnap != nil {
		toSchema = toSnap.Schema
		toData = toSnap.Data
	}

	// 1. 运行 DDL 差异比对
	tableDDLs, ddlSummary := DiffSchemas(fromSchema, toSchema)

	// 2. 运行 DML 差异比对
	tableDMLs, dmlSummary := DiffData(fromData, toData)

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
		dmlScriptBuilder.WriteString("\n")
	}

	now := time.Now()
	res := &model.DiffResult{
		ConnectionID:  connID,
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
	fullBuilder.WriteString("-- MySQL Monitor 版本差异同步脚本\n")
	fullBuilder.WriteString(fmt.Sprintf("-- 连接标识: %s\n", connID))
	fullBuilder.WriteString(fmt.Sprintf("-- 起始基准版本: %s\n", fromVer))
	fullBuilder.WriteString(fmt.Sprintf("-- 目标对比版本: %s\n", toVer))
	fullBuilder.WriteString(fmt.Sprintf("-- 生成时间: %s\n", now.Format("2006-01-02 15:04:05")))
	fullBuilder.WriteString("--\n")
	fullBuilder.WriteString(fmt.Sprintf("-- DDL 变动: 新建表 %d, 删除表 %d, 变更表 %d | 新增字段 %d, 删除字段 %d, 修改字段 %d | 新增索引 %d, 删除索引 %d\n",
		ddlSummary.AddedTables, ddlSummary.DroppedTables, ddlSummary.ModifiedTables,
		ddlSummary.AddedColumns, ddlSummary.DroppedColumns, ddlSummary.ModifiedColumns,
		ddlSummary.AddedIndexes, ddlSummary.DroppedIndexes))
	fullBuilder.WriteString(fmt.Sprintf("-- DML 变动: 插入数据 %d 行, 修改数据 %d 行, 删除数据 %d 行\n",
		dmlSummary.InsertCount, dmlSummary.UpdateCount, dmlSummary.DeleteCount))
	fullBuilder.WriteString("-- ====================================================================\n\n")

	fullBuilder.WriteString("SET NAMES utf8mb4;\n")
	fullBuilder.WriteString("SET FOREIGN_KEY_CHECKS = 0;\n\n")

	if res.DDLScript != "" {
		fullBuilder.WriteString("-- --------------------------------------------------------------------\n")
		fullBuilder.WriteString("-- 1. DDL 结构变更\n")
		fullBuilder.WriteString("-- --------------------------------------------------------------------\n")
		fullBuilder.WriteString(res.DDLScript)
		fullBuilder.WriteString("\n\n")
	}

	if res.DMLScript != "" {
		fullBuilder.WriteString("-- --------------------------------------------------------------------\n")
		fullBuilder.WriteString("-- 2. DML 数据变更（事务保护）\n")
		fullBuilder.WriteString("-- --------------------------------------------------------------------\n")
		fullBuilder.WriteString("START TRANSACTION;\n\n")
		fullBuilder.WriteString(res.DMLScript)
		fullBuilder.WriteString("\n\nCOMMIT;\n\n")
	}

	fullBuilder.WriteString("SET FOREIGN_KEY_CHECKS = 1;\n")
	res.FullScript = strings.TrimSpace(fullBuilder.String())

	return res
}

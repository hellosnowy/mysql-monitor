# MySQL Monitor (DDL & DML 监控与版本比对工具)

基于 **Golang** 与 **Wails v2** 构建的跨平台桌面应用，用于监控 MySQL 数据库结构 (DDL) 与数据 (DML) 变动、捕获历史版本基线 (Baseline)，并支持在任意两个历史版本之间进行智能差异比对与 SQL 脚本导出。

---

## 核心特性

1. **多数据库连接管理**
   - 支持添加、修改、测试与删除多个 MySQL 数据源配置。
   - 实时连通性探测（包含延迟测速与 MySQL 服务端版本检测）。
   - 零 CGO 依赖，纯 Go 跨平台支持。

2. **灵活的表黑白名单过滤**
   - **白名单 (IncludeTables)**：支持精确表名 (`t_user`)、Glob 通配符 (`t_order_*`) 与正则表达式 (`^tbl_\d+$`)。
   - **黑名单 (ExcludeTables)**：优先级高于白名单，命中即排除（如 `*_bak`, `tmp_*`）。
   - **监控模式选择**：支持 `结构与数据全量监控 (DDL + DML)` 或 `仅监控表结构 (DDL)`。
   - **大表防护**：支持针对单表设置最大抓取行数限制。

3. **基准版本 (Baseline) 与历史快照**
   - **智能基准确立**：若当前库历史版本为空，首次拉取自动确立为【基准版本 (Baseline)】。
   - 支持后续手动指定或覆盖新基准版本。
   - 包含表结构 (`information_schema` + `SHOW CREATE TABLE`) 与行数据快照。

4. **任意两历史版本双向比对与 SQL 生成**
   - **DDL 差异比对**：
     - 新建表 (`CREATE TABLE`) 与删除表 (`DROP TABLE IF EXISTS`)。
     - 字段变更：新增字段 (`ADD COLUMN ... AFTER ...`)、删除字段 (`DROP COLUMN`)、修改字段 (`MODIFY COLUMN`)。
     - 索引变动：新增索引、删除索引、主键变更 (`DROP PRIMARY KEY, ADD PRIMARY KEY`)。
     - 存储引擎及注释更新。
   - **DML 差异比对**：
     - 有主键时按主键对齐行；无主键时按整行内容和重复次数比对。
     - 无主键重复行的删除语句使用 MySQL `LIMIT 1`，每次只删除一行。
     - 自动生成 `INSERT INTO ...`、`DELETE FROM ... WHERE ...` 及 `UPDATE ... SET ... WHERE ...`。
     - 完善的字符转义、NULL 处理与二进制/十六进制输出。
   - **SQL 导出与事务保护**：
     - 生成带有 `START TRANSACTION; ... COMMIT;` 以及 `FOREIGN_KEY_CHECKS` 保护的完整同步脚本。
     - 支持界面一键复制与系统原生对话框导出为 `.sql` 文件。

---

## 目录架构

```
mysql-monitor/
├── main.go                  # Wails 桌面应用入口（嵌入前端 dist）
├── app.go                   # Wails 前后端桥接服务（供前端调用的 Go API）
├── wails.json               # Wails 项目配置文件
├── internal/
│   ├── model/               # 领域实体模型 (连接、结构、数据、快照、差异)
│   ├── filter/              # 表黑白名单过滤器
│   ├── db/                  # MySQL 客户端、元数据与数据抽取器
│   ├── storage/             # 本地结构化快照持久化仓储
│   ├── diff/                # DDL 与 DML 核心比对与 SQL 生成引擎
│   └── service/             # 业务编排服务
├── pkg/
│   └── sqlutil/             # SQL 转义、字符串与主键处理工具
└── frontend/                # Vue 3 + Vite 现代化前端工程
    ├── src/
    │   ├── App.vue          # 主页面（连接管理、快照管理、差异比对）
    │   ├── api.js           # Wails Go 方法调用封装
    │   └── style.css        # 样式定义
    ├── package.json
    └── vite.config.js
```

---

## 构建与运行

### 1. 前提环境
- Go 1.20+（已在 Go 1.22 验证）
- Node.js 18+ 与 npm

### 2. 本地开发
```bash
# 1. 运行前端开发服务（可选单独调试）
cd frontend
npm install
npm run dev

# 2. 或使用 Wails 开发模式（前后端热重载）
wails dev
```

### 3. 编译打包独立可执行文件
推荐使用官方 Wails CLI 构建（自动处理 Windows Manifest、图标与生产构建标签）：
```bash
# 执行完整打包构建（产物位于 build/bin/mysql-monitor.exe 及 bin/mysql-monitor.exe）
wails build
```

或使用标准 Go 工具链手动编译（注意必须指定 `-tags "production"`）：
```bash
# 1. 构建前端资源
cd frontend && npm run build && cd ..

# 2. 编译生产二进制 (windowsgui 隐藏后台命令行黑色窗口)
go build -tags "production" -ldflags "-H windowsgui -s -w" -o bin/mysql-monitor.exe .
```

### 4. 运行单元测试
```bash
go test -v ./...
```

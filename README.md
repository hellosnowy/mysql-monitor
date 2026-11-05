# SQL Monitor (多数据库 DDL & DML 监控与版本比对工具)

基于 **Golang** 与 **Wails v2** 构建的跨平台桌面应用，用于监控 **MySQL、PostgreSQL、SQL Server** 数据库结构 (DDL) 与数据 (DML) 变动、捕获历史版本基线 (Baseline)，并支持在任意两个历史版本之间生成 **升级** 或 **回滚** SQL 脚本。

---

## 核心特性

1. **多数据库连接管理**
   - 支持 MySQL / PostgreSQL / SQL Server，内置三家纯 Go 驱动（零 CGO）。
   - 连接配置可选择厂商、目标数据库与 Schema/模式（PostgreSQL 默认 `public`，SQL Server 默认 `dbo`）。
   - 支持从服务端加载 Schema 列表；实时连通性探测（延迟 + 服务端版本）。
   - 老版本 `mysql-monitor` 的 `./data` 配置与快照可直接读取，缺失厂商字段自动按 MySQL 兼容。

2. **方言插件架构**
   - `internal/dialect` 定义统一的 `Plugin` 接口：连接、读取快照、比对、生成 SQL。
   - `internal/dialect/{mysql,postgres,sqlserver}` 各自实现元数据提取与 SQL 发射。
   - `internal/diff` 只负责“改了什么”，SQL 语法由各方言的 `sqlgen.Emitter` 决定。

3. **基准版本 (Baseline) 与历史快照**
   - 首次抓取自动确立基准版本，后续可手动指定。
   - 快照包含表结构、索引、主键、注释与行数据；版本号精确到毫秒且禁止覆盖已有版本。

4. **升级 / 回滚双向比对**
   - **升级 (From → To)**：生成由起始版本升级到目标版本的增量 SQL。
   - **回滚 (To → From)**：生理由目标版本回滚到起始版本的 SQL。
   - DDL：建表/删表、字段增删改、主键与索引变更、表/列注释。
   - DML：有主键按主键对齐，无主键按整行内容与重复次数比对；生成 INSERT/UPDATE/DELETE。
   - 完整脚本按方言包裹事务：MySQL `START TRANSACTION`、PostgreSQL `BEGIN/COMMIT`、SQL Server `SET XACT_ABORT ON + BEGIN TRANSACTION`。
   - 支持界面复制与原生对话框导出 `.sql`。

---

## 支持矩阵与已知限制

| 能力 | MySQL | PostgreSQL | SQL Server |
| --- | --- | --- | --- |
| 连接 / 版本探测 | ✅ | ✅ | ✅ |
| Schema 选择 | 数据库名即 schema | ✅ `public` 等 | ✅ `dbo` 等 |
| 表/字段/主键/索引快照 | ✅ | ✅ | ✅ |
| 原生建表语句 | ✅ `SHOW CREATE TABLE` | 由元数据重建 | 由元数据重建 |
| 列位置保持 (`FIRST/AFTER`) | ✅ | 仅追加 | 仅追加 |
| Identity/自增插入 | 普通 INSERT | `OVERRIDING SYSTEM VALUE` | `SET IDENTITY_INSERT` |
| 无主键重复行删除 | `LIMIT 1` | `ctid` 子查询 | `DELETE TOP (1)` |
| 外键 / 视图 / 存储过程 / 触发器 | ❌ | ❌ | ❌ |

- 本次不建模外键、视图、存储过程、触发器、序列状态、枚举/自定义类型与分区表；含外键的库可能需要手工调整脚本执行顺序。
- PostgreSQL 的 `serial` 列在建表时按 `serial/bigserial` 重建；其他默认值保留表达式。
- SQL Server 修改默认约束依赖快照中记录的约束名；老快照缺失时会退化为匿名 `ADD DEFAULT`。
- 显式插入 identity/sequence 值后，序列计数器不会自动推进。
- 跨方言快照不可互相比较；已有历史快照的连接禁止修改厂商。

---

## 目录架构

```
sql-monitor/
├── main.go                       # Wails 桌面应用入口（嵌入前端 dist）
├── app.go                        # Wails 前后端桥接服务
├── wails.json                    # Wails 项目配置
├── internal/
│   ├── model/                    # 领域模型 + 老版本数据归一化
│   ├── sqlgen/                   # 方言 SQL 发射接口
│   ├── diff/                     # 与方言无关的 DDL/DML 比对引擎
│   ├── dialect/
│   │   ├── dialect.go            # Plugin 接口、注册表与通用抓取/比对流程
│   │   ├── builtin/              # 内置方言注册（组合根）
│   │   ├── mysql/                # MySQL 方言插件
│   │   ├── postgres/             # PostgreSQL 方言插件
│   │   └── sqlserver/            # SQL Server 方言插件
│   ├── db/                       # 跨方言 rows.Scan 辅助
│   ├── filter/                   # 表黑白名单过滤器
│   ├── storage/                  # 本地快照持久化（兼容老格式）
│   └── service/                  # 业务编排
├── pkg/
│   ├── crypto/                   # 连接密码 AES 加密
│   └── sqlutil/                  # MySQL 转义/字面量工具
└── frontend/                     # Vue 3 + Vite
    └── src/
        ├── App.vue               # 连接 / 快照 / 比对界面
        └── api.js                # Wails 调用封装（含浏览器 Mock）
```

---

## 构建与运行

### 1. 前提环境
- Go 1.25+
- Node.js 18+ 与 npm
- （可选）Wails CLI：`go install github.com/wailsapp/wails/v2/cmd/wails@latest`

### 2. 本地开发
```bash
cd frontend
npm install
npm run dev
```

或使用 Wails 开发模式（前后端热重载）：
```bash
wails dev
```

### 3. 编译打包
```bash
wails build
```

或使用标准 Go 工具链（必须指定 `-tags "production"`）：
```bash
cd frontend && npm run build && cd ..
go build -tags "production" -ldflags "-H windowsgui -s -w" -o bin/sql-monitor.exe .
```

### 4. 运行单元测试
```bash
go test ./...
```

### 5. 数据目录与升级
- 数据仍保存在程序工作目录下的 `./data`（`configs/connections.json`、`snapshots/<connID>/<version>/`）。
- 从 `mysql-monitor` 升级时，将 `sql-monitor.exe` 放到原目录运行即可；旧配置与快照无需迁移，读取时自动补 `vendor=mysql`。
- 数据目录包含加密密钥 `.secret_key`，请勿删除，否则已保存的密码无法解密。

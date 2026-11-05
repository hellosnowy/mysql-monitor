export namespace dialect {
	
	export class Capabilities {
	    schema_namespace: boolean;
	    identity_insert: boolean;
	    transactional_ddl: boolean;
	    table_comments: boolean;
	    column_comments: boolean;
	    partial_indexes: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Capabilities(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.schema_namespace = source["schema_namespace"];
	        this.identity_insert = source["identity_insert"];
	        this.transactional_ddl = source["transactional_ddl"];
	        this.table_comments = source["table_comments"];
	        this.column_comments = source["column_comments"];
	        this.partial_indexes = source["partial_indexes"];
	    }
	}
	export class DriverInfo {
	    vendor: string;
	    display_name: string;
	    driver_package: string;
	    driver_version: string;
	    built_in: boolean;
	    default_port: number;
	    default_schema: string;
	    supports_schemas: boolean;
	    capabilities: Capabilities;
	
	    static createFrom(source: any = {}) {
	        return new DriverInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.vendor = source["vendor"];
	        this.display_name = source["display_name"];
	        this.driver_package = source["driver_package"];
	        this.driver_version = source["driver_version"];
	        this.built_in = source["built_in"];
	        this.default_port = source["default_port"];
	        this.default_schema = source["default_schema"];
	        this.supports_schemas = source["supports_schemas"];
	        this.capabilities = this.convertValues(source["capabilities"], Capabilities);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace model {
	
	export class TableFilterConfig {
	    include_tables: string[];
	    exclude_tables: string[];
	    monitor_mode: string;
	    max_rows_per_table: number;
	
	    static createFrom(source: any = {}) {
	        return new TableFilterConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.include_tables = source["include_tables"];
	        this.exclude_tables = source["exclude_tables"];
	        this.monitor_mode = source["monitor_mode"];
	        this.max_rows_per_table = source["max_rows_per_table"];
	    }
	}
	export class ConnectionConfig {
	    id: string;
	    name: string;
	    vendor?: string;
	    host: string;
	    port: number;
	    user: string;
	    password: string;
	    database: string;
	    schema?: string;
	    charset?: string;
	    ssl_mode?: string;
	    options?: Record<string, string>;
	    timeout_seconds: number;
	    filter: TableFilterConfig;
	    // Go type: time
	    created_at: any;
	    // Go type: time
	    updated_at: any;
	
	    static createFrom(source: any = {}) {
	        return new ConnectionConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.vendor = source["vendor"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.user = source["user"];
	        this.password = source["password"];
	        this.database = source["database"];
	        this.schema = source["schema"];
	        this.charset = source["charset"];
	        this.ssl_mode = source["ssl_mode"];
	        this.options = source["options"];
	        this.timeout_seconds = source["timeout_seconds"];
	        this.filter = this.convertValues(source["filter"], TableFilterConfig);
	        this.created_at = this.convertValues(source["created_at"], null);
	        this.updated_at = this.convertValues(source["updated_at"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ConnectionTestResult {
	    success: boolean;
	    message: string;
	    server_version: string;
	    latency_ms: number;
	
	    static createFrom(source: any = {}) {
	        return new ConnectionTestResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.success = source["success"];
	        this.message = source["message"];
	        this.server_version = source["server_version"];
	        this.latency_ms = source["latency_ms"];
	    }
	}
	export class DDLDiffSummary {
	    added_tables: number;
	    dropped_tables: number;
	    modified_tables: number;
	    added_columns: number;
	    dropped_columns: number;
	    modified_columns: number;
	    added_indexes: number;
	    dropped_indexes: number;
	
	    static createFrom(source: any = {}) {
	        return new DDLDiffSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.added_tables = source["added_tables"];
	        this.dropped_tables = source["dropped_tables"];
	        this.modified_tables = source["modified_tables"];
	        this.added_columns = source["added_columns"];
	        this.dropped_columns = source["dropped_columns"];
	        this.modified_columns = source["modified_columns"];
	        this.added_indexes = source["added_indexes"];
	        this.dropped_indexes = source["dropped_indexes"];
	    }
	}
	export class DMLDiffSummary {
	    insert_count: number;
	    update_count: number;
	    delete_count: number;
	
	    static createFrom(source: any = {}) {
	        return new DMLDiffSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.insert_count = source["insert_count"];
	        this.update_count = source["update_count"];
	        this.delete_count = source["delete_count"];
	    }
	}
	export class TableDMLDiff {
	    table_name: string;
	    insert_statements: string[];
	    update_statements: string[];
	    delete_statements: string[];
	    insert_count: number;
	    update_count: number;
	    delete_count: number;
	
	    static createFrom(source: any = {}) {
	        return new TableDMLDiff(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.table_name = source["table_name"];
	        this.insert_statements = source["insert_statements"];
	        this.update_statements = source["update_statements"];
	        this.delete_statements = source["delete_statements"];
	        this.insert_count = source["insert_count"];
	        this.update_count = source["update_count"];
	        this.delete_count = source["delete_count"];
	    }
	}
	export class TableDDLDiff {
	    table_name: string;
	    diff_type: string;
	    statements: string[];
	    details: string[];
	
	    static createFrom(source: any = {}) {
	        return new TableDDLDiff(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.table_name = source["table_name"];
	        this.diff_type = source["diff_type"];
	        this.statements = source["statements"];
	        this.details = source["details"];
	    }
	}
	export class DiffSummary {
	    ddl: DDLDiffSummary;
	    dml: DMLDiffSummary;
	
	    static createFrom(source: any = {}) {
	        return new DiffSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ddl = this.convertValues(source["ddl"], DDLDiffSummary);
	        this.dml = this.convertValues(source["dml"], DMLDiffSummary);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class DiffResult {
	    connection_id: string;
	    vendor?: string;
	    direction?: string;
	    from_version_id: string;
	    to_version_id: string;
	    // Go type: time
	    compared_at: any;
	    summary: DiffSummary;
	    table_ddl_diffs: TableDDLDiff[];
	    table_dml_diffs: TableDMLDiff[];
	    ddl_script: string;
	    dml_script: string;
	    full_script: string;
	
	    static createFrom(source: any = {}) {
	        return new DiffResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.connection_id = source["connection_id"];
	        this.vendor = source["vendor"];
	        this.direction = source["direction"];
	        this.from_version_id = source["from_version_id"];
	        this.to_version_id = source["to_version_id"];
	        this.compared_at = this.convertValues(source["compared_at"], null);
	        this.summary = this.convertValues(source["summary"], DiffSummary);
	        this.table_ddl_diffs = this.convertValues(source["table_ddl_diffs"], TableDDLDiff);
	        this.table_dml_diffs = this.convertValues(source["table_dml_diffs"], TableDMLDiff);
	        this.ddl_script = source["ddl_script"];
	        this.dml_script = source["dml_script"];
	        this.full_script = source["full_script"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	
	export class VersionMeta {
	    version_id: string;
	    connection_id: string;
	    vendor?: string;
	    database_name: string;
	    schema?: string;
	    format_version?: number;
	    is_baseline: boolean;
	    description: string;
	    table_count: number;
	    row_count: number;
	    // Go type: time
	    created_at: any;
	
	    static createFrom(source: any = {}) {
	        return new VersionMeta(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version_id = source["version_id"];
	        this.connection_id = source["connection_id"];
	        this.vendor = source["vendor"];
	        this.database_name = source["database_name"];
	        this.schema = source["schema"];
	        this.format_version = source["format_version"];
	        this.is_baseline = source["is_baseline"];
	        this.description = source["description"];
	        this.table_count = source["table_count"];
	        this.row_count = source["row_count"];
	        this.created_at = this.convertValues(source["created_at"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}


import os
import re
import sys
import mysql.connector
from urllib.parse import urlparse, parse_qs

def parse_database_url(url_str):
    parsed = urlparse(url_str)
    return {
        'host': parsed.hostname or '127.0.0.1',
        'port': parsed.port or 3306,
        'user': parsed.username or 'root',
        'password': parsed.password or '',
        'database': parsed.path.lstrip('/') or 'coal_governance'
    }

def get_connection_params():
    db_url = os.environ.get('DATABASE_URL')
    if db_url:
        params = parse_database_url(db_url)
    else:
        params = {
            'host': os.environ.get('DB_HOST', '127.0.0.1'),
            'port': int(os.environ.get('DB_PORT', '3306')),
            'user': os.environ.get('DB_USER', 'root'),
            'password': os.environ.get('DB_PASSWORD', ''),
            'database': os.environ.get('DB_NAME', 'coal_governance')
        }
    
    ssl_mode = os.environ.get('DB_SSL_MODE', '').lower()
    if ssl_mode in ('require', 'true', 'skip-verify') or (params['host'] not in ('127.0.0.1', 'localhost', 'mysql')):
        params['ssl_disabled'] = False
    
    return params

def run_sql_file(cursor, file_path, target_db):
    if not os.path.exists(file_path):
        print(f"[Warning] SQL file not found: {file_path}")
        return

    print(f"[Info] Executing SQL script: {file_path}")
    with open(file_path, 'r', encoding='utf-8') as f:
        content = f.read()

    # Split into statements by semicolon followed by newline
    statements = re.split(r';\s*\n', content)
    for raw_stmt in statements:
        stmt = raw_stmt.strip()
        if not stmt:
            continue
        
        # Skip comments
        if stmt.startswith('--') or stmt.startswith('/*'):
            # Check if there is actual sql after comments
            lines = [l for l in stmt.splitlines() if not l.strip().startswith('--')]
            stmt = "\n".join(lines).strip()
            if not stmt:
                continue

        # Skip CREATE DATABASE / USE coal_governance if target_db is already selected
        if re.match(r'^CREATE\s+DATABASE', stmt, re.IGNORECASE) or re.match(r'^USE\s+', stmt, re.IGNORECASE):
            continue

        try:
            cursor.execute(stmt)
        except mysql.connector.Error as err:
            # Ignore table already exists or duplicate key errors during idempotent migrations
            if err.errno in (1050, 1060, 1061, 1062, 1068):
                continue
            # Also ignore foreign key already exists (1826)
            if err.errno == 1826:
                continue
            print(f"[Warning] SQL Error ({err.errno}) on: {stmt[:80]}... -> {err.msg}")

def main():
    params = get_connection_params()
    print(f"[Info] Connecting to MySQL at {params['host']}:{params['port']} as {params['user']}...")

    # First attempt: connect directly to database
    conn = None
    target_db = params.get('database', 'coal_governance')
    try:
        conn = mysql.connector.connect(**params)
        print(f"[Success] Connected directly to database: {target_db}")
    except mysql.connector.Error as err:
        # If unknown database, try connecting to server root and creating it
        if err.errno == 1049: # Unknown database
            print(f"[Info] Database '{target_db}' does not exist yet. Connecting without database to create it...")
            init_params = {k: v for k, v in params.items() if k != 'database'}
            try:
                root_conn = mysql.connector.connect(**init_params)
                root_cur = root_conn.cursor()
                root_cur.execute(f"CREATE DATABASE IF NOT EXISTS `{target_db}` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;")
                root_conn.commit()
                root_cur.close()
                root_conn.close()
                print(f"[Success] Database '{target_db}' created.")
                conn = mysql.connector.connect(**params)
            except Exception as inner_err:
                print(f"[Error] Failed to create database: {inner_err}")
                sys.exit(1)
        else:
            print(f"[Error] Failed to connect to MySQL: {err}")
            sys.exit(1)

    cursor = conn.cursor()
    base_dir = os.path.dirname(os.path.abspath(__file__))
    schema_path = os.path.join(base_dir, 'schema.sql')
    seed_path = os.path.join(base_dir, 'seed.sql')

    print("[Info] Applying schema...")
    run_sql_file(cursor, schema_path, target_db)
    conn.commit()
    print("[Success] Schema applied successfully.")

    print("[Info] Applying seed data...")
    run_sql_file(cursor, seed_path, target_db)
    conn.commit()
    print("[Success] Seed data applied successfully.")

    # Verification: check table count and key tables
    cursor.execute("SHOW TABLES;")
    tables = [row[0] for row in cursor.fetchall()]
    print(f"[Verification] Total tables verified: {len(tables)}")
    for t in ['users', 'mines', 'inspections', 'violations', 'corrective_actions', 'compliance_rules']:
        if t in tables:
            cursor.execute(f"SELECT COUNT(*) FROM `{t}`;")
            count = cursor.fetchone()[0]
            print(f"  - Table `{t}`: {count} rows")
        else:
            print(f"  - Table `{t}`: MISSING")

    cursor.close()
    conn.close()
    print("[Done] Database migration and verification complete!")

if __name__ == '__main__':
    main()

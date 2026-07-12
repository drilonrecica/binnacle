CREATE TABLE host_rollups_1m (ts INTEGER PRIMARY KEY,cpu_avg REAL,cpu_min REAL,cpu_max REAL,sample_count INTEGER NOT NULL);
CREATE TABLE resource_rollups_1m (ts INTEGER NOT NULL,resource_id TEXT NOT NULL,cpu_avg REAL,cpu_min REAL,cpu_max REAL,sample_count INTEGER NOT NULL,PRIMARY KEY(resource_id,ts));

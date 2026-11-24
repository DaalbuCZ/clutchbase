ALTER TABLE machines ADD COLUMN cpu_percent double precision,
 ADD COLUMN memory_total bigint, ADD COLUMN memory_available bigint,
 ADD CONSTRAINT machines_cpu_percent_check CHECK (cpu_percent >= 0 AND cpu_percent <= 100),
 ADD CONSTRAINT machines_memory_check CHECK ((memory_total IS NULL AND memory_available IS NULL) OR (memory_total IS NOT NULL AND memory_available IS NOT NULL AND memory_total>0 AND memory_available>=0 AND memory_available<=memory_total));

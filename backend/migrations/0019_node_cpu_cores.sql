-- The panel had no idea how many CPU cores a node's actual host machine
-- has -- memory_mb/disk_mb are admin-set allocatable capacity, but there
-- was no equivalent for CPU at all, so setting a per-server CPU limit had
-- nothing to check against. Self-reported by wingsd (runtime.NumCPU()) on
-- every health check, same pattern as agent_version -- not admin-set, since
-- unlike memory/disk (which are deliberately allocatable capacity the
-- admin controls for oversell), core count is just a hardware fact.
ALTER TABLE nodes ADD COLUMN total_cpu_cores INT;

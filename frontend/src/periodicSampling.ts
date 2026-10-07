import type {NodeOption} from './domain'
export interface PeriodicSamplingConfig {
 latency_probe_set?:string
 enabled:boolean
 latency_interval_seconds:number
 service_interval_seconds:number
 download_interval_seconds:number
 download_mib:number
 service_concurrency?:number
 include_antigravity:boolean
 selections?:NodeOption[]
}
export interface PeriodicCycle {
 timing_version?:number;queue_wait_seconds?:number;download_exclusion_wait_seconds?:number;admission_wait_seconds?:number;network_active?:number;legacy_observation?:boolean;started?:number;active?:number;waiting?:number;not_executed?:number;execution_seconds?:number;estimated_total_seconds?:number|null;remaining_seconds?:number|null;concurrency?:number;elapsed_seconds?:number;wait_seconds?:number;covered_nodes?:number;covered_services?:number;completed_nodes?:number;full_coverage?:boolean;interrupted?:boolean
 running:boolean;total:number;processed:number;saved:number;unsaved:number;bytes_read:number
 next_due:string;finished_at:string;skipped_slots:number;error?:string;outcomes:Record<string,number>
}
export interface PeriodicSamplingStatus {
 config:PeriodicSamplingConfig;running:boolean;node_count:number;service_count:number
 recent_cycles?:{kind:string;cycle:PeriodicCycle}[]
 monitor_job_ids:string[];pause_reason?:string;error?:string;cycles:Record<string,PeriodicCycle>
}

export interface PeriodicServiceCheck {service_id:string;name:string;last_detected_at:string|null;execution:string;persistence:string;outcome:string;competing_probes?:string}
export interface PeriodicChecks {nodes:NodeOption[];node:NodeOption;checks:PeriodicServiceCheck[]}

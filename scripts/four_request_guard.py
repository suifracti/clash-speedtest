"""Observer for an explicitly authorized two-node / four-request native run.

Usage: python3 scripts/four_request_guard.py /absolute/fresh/evidence-directory
Never starts HTTP or clicks UI. The native plan must enforce request limits;
SQLite observation is after-the-fact protection, not a packet/request firewall. Input files and arming remain explicit. Read-only
SQLite; SIGTERM is reserved for a still-identical isolated process at a safety
boundary. A saved terminal round ends observation without closing the App.
Do not point this at a previous run: its evidence must remain immutable.
"""
import os,json,time,signal,sqlite3,subprocess,hashlib,re,math
from pathlib import Path
from contextlib import closing
import sys

class GateViolation(RuntimeError):
 pass

def require(condition, message):
 if not condition:raise GateViolation(message)

r=Path(sys.argv[1]).resolve()
if (r/'run-boundary.json').exists():
 raise SystemExit('refusing to overwrite an existing run')
entry=json.loads((r/'candidate-entry.json').read_text()); root=Path(entry['data_root']);core=entry['core']
def matching_process(command):
 command=command.strip()
 if not command.startswith(core+' '):return False
 if not re.search(r'(?:^|\s)--gui(?:\s|$)',command):return False
 if not re.search(r'(?:^|\s)--no-auto-credentials(?:\s|$)',command):return False
 roots=re.findall(r'(?:^|\s)--data-dir(?:=|\s+)(.*?)(?=\s--[A-Za-z]|$)',command)
 return len(roots)==1 and roots[0].strip()==str(root)

def terminate_verified():
 try:
  current=subprocess.check_output(['ps','-ww','-p',str(pid),'-o','command='],timeout=2).decode()
  if not matching_process(current):return False
  os.kill(pid,signal.SIGTERM)
  return True
 except (subprocess.SubprocessError,ProcessLookupError):
  return False

ps=subprocess.check_output(['ps','-axww','-o','pid=,command=']).decode().splitlines()
ps=[x.strip().split(maxsplit=1) for x in ps if len(x.strip().split(maxsplit=1))==2 and matching_process(x.strip().split(maxsplit=1)[1])]
require(len(ps)==1,'single isolated writer gate failed')
pid=int(ps[0][0]);require('--no-auto-credentials' in ps[0][1], 'automatic credentials must be disabled')
require(hashlib.sha256(Path(core).read_bytes()).hexdigest()==json.loads((r/'build-manifest.json').read_text())['core_sha256'], 'candidate binary mismatch')
verified=json.loads((r/'verified-nodes.json').read_text());expected={(x['profile_id'],x['identity_sha256'],x['revision']) for x in verified}
require(len(verified)==2 and len(expected)==2, 'exactly two frozen nodes required')
def db():
 d=sqlite3.connect('file:'+str(root/'history/history.db')+'?mode=ro',uri=True,timeout=1);d.row_factory=sqlite3.Row;return d
with closing(db()) as d:
 tables=['workbench_latency_tests','workbench_latency_samples','workbench_latency_batches','workbench_latency_batch_items','workbench_download_attempts','workbench_download_results','workbench_public_service_attempts','workbench_public_service_results','measurement_rounds','measurement_round_items','monitor_samples','monitor_job_definitions']
 baseline={t:{'count':d.execute('select count(*) from '+t).fetchone()[0],'row_hashes':sorted(hashlib.sha256(json.dumps(dict(x),sort_keys=True,default=str).encode()).hexdigest() for x in d.execute('select * from '+t))} for t in tables}
 old_rounds={x[0] for x in d.execute('select round_id from measurement_rounds')}
 old_latency={x[0] for x in d.execute('select attempt_id from workbench_latency_tests')};old_service={x[0] for x in d.execute('select attempt_id from workbench_public_service_attempts')}
 require(d.execute("select count(*) from measurement_rounds where state!='finished'").fetchone()[0]==0, "unfinished round exists")
 require(d.execute('select count(*) from monitor_job_definitions').fetchone()[0]==0, 'monitor definitions exist')
(r/'run-baseline.json').write_text(json.dumps(baseline,indent=2))
launch={'pid':pid,'root':str(root),'core':core};(r/'live-launch.json').write_text(json.dumps(launch,indent=2))
boundary={'pid':pid,'state':'ready','maximum_network_seconds':180,'max_latency_samples':2,'max_service_attempts':2,'max_download_attempts':0,'max_target_attempts':4,'service_allowlist':['cloudflare_204'],'latency_timeout_seconds':10,'service_timeout_seconds':15,'stop_reason':None};(r/'run-boundary.json').write_text(json.dumps(boundary,indent=2))

def all_four_saved(d):
 rounds=[x for x in d.execute('select round_id,state from measurement_rounds') if x['round_id'] not in old_rounds]
 if len(rounds)!=1 or rounds[0]['state']!='finished':return False
 items=list(d.execute('select * from measurement_round_items where round_id=?',(rounds[0]['round_id'],)))
 if len(items)!=4:return False
 found=set()
 for item in items:
  scope=(item['profile_id'],item['node_key'],item['node_identity_key'],item['config_revision_key'])
  identity=(scope[0],hashlib.sha256(scope[2].encode()).hexdigest(),scope[3])
  if identity not in expected or item['not_executed_reason']:return False
  found.add((identity,item['project']))
  if item['project']=='latency':
   row=d.execute("""select i.execution_state,i.persistence_state,t.attempt_id as saved_id
    from workbench_latency_batches b join workbench_latency_batch_items i on i.batch_id=b.batch_id
    left join workbench_latency_tests t on t.attempt_id=i.attempt_id
    where b.request_id=? and i.profile_id=? and i.node_key=? and i.node_identity_key=? and i.config_revision_key=?""",(item['request_id'],*scope)).fetchone()
  elif item['project']=='service' and item['service_id']=='cloudflare_204':
   row=d.execute("""select a.execution_state,a.persistence_state,r.attempt_id as saved_id
    from workbench_public_service_attempts a left join workbench_public_service_results r on r.attempt_id=a.attempt_id
    where a.request_id=? and a.profile_id=? and a.node_key=? and a.node_identity_key=? and a.config_revision_key=? and a.service_id=?""",(item['request_id'],*scope,item['service_id'])).fetchone()
  else:return False
  if not row or row['execution_state'] not in ('completed','failed','cancelled','not_executed') or row['persistence_state']!='saved' or not row['saved_id']:return False
 return found=={(identity,project) for identity in expected for project in ('latency','service')}

print('GUARD_READY pid='+str(pid),flush=True)
events=[];deadline=None;created=time.time();failures=0
while True:
 reason=None
 if (r/'stop-live-guard').exists():reason='operator_stop'
 elif not deadline and (r/'arm-live-guard.json').exists():
  try:
   arm=json.loads((r/'arm-live-guard.json').read_text());armed=float(arm['armed_at_epoch'])
   require(math.isfinite(armed) and armed<=time.time(),'invalid arm time')
   deadline=armed+180;boundary.update(state='armed',started_epoch=armed,absolute_deadline_epoch=deadline)
  except (OSError,ValueError,TypeError,KeyError,GateViolation):reason='invalid_arm'
 elif not deadline and time.time()-created>600:reason='prestart_timeout'
 if reason is None and deadline and time.time()>=deadline:reason='absolute_deadline'
 try:
  command=subprocess.check_output(['ps','-ww','-p',str(pid),'-o','command=']).decode()
  if not matching_process(command):reason='process_identity_changed'
 except subprocess.CalledProcessError:reason='process_exited'
 if reason:
  if reason not in ['process_exited','prestart_timeout']:
   boundary['signal_sent']=terminate_verified()
  boundary.update(state='stopped',stop_reason=reason,finished_epoch=time.time());(r/'run-boundary.json').write_text(json.dumps(boundary,indent=2));break
 try:
  with closing(db()) as d:
   d.execute('BEGIN')  # one consistent read snapshot for counts, scope and completion
   counts={t:d.execute('select count(*) from '+t).fetchone()[0] for t in tables};delta={t:counts[t]-baseline[t]['count'] for t in tables}
   require(delta['workbench_latency_samples']<=2 and delta['workbench_latency_tests']<=2 and delta['workbench_public_service_attempts']<=2 and delta['workbench_download_attempts']==0 and counts['monitor_job_definitions']==0 and counts['monitor_samples']==0,'budget/scope gate violation')
   for t,old in [('workbench_latency_tests',old_latency),('workbench_public_service_attempts',old_service)]:
    for a in d.execute('select * from '+t):
     if a['attempt_id'] in old:continue
     require((a['profile_id'],hashlib.sha256(a['node_identity_key'].encode()).hexdigest(),a['config_revision_key']) in expected,'node scope drift')
     if t.endswith('service_attempts'):require(a['service_id']=='cloudflare_204','unexpected service')
   for sample in d.execute('select s.attempt_id,s.target from workbench_latency_samples s'):
    if sample['attempt_id'] not in old_latency:require(sample['target']=='https://speed.cloudflare.com/__down?bytes=1','unexpected target')
   complete=bool(deadline and all_four_saved(d))
   events.append({'at':time.time(),'delta':delta,'all_four_saved':complete})
  failures=0
  if complete:
   boundary.update(state='stopped',stop_reason='completed_saved',finished_epoch=time.time())
   break
 except Exception as e:
  failures+=1;events.append({'at':time.time(),'observer_error':type(e).__name__,'consecutive':failures})
  if isinstance(e,GateViolation) or failures>=3:
   boundary['signal_sent']=terminate_verified();boundary.update(state='stopped',stop_reason='budget_or_observer_gate',finished_epoch=time.time());break
 try:
  sockets=subprocess.run(['lsof','-nP','-a','-p',str(pid),'-i'],capture_output=True,text=True,timeout=2).stdout
  with (r/'private/live-sockets.log').open('a') as f:f.write(str(time.time())+'\n'+sockets)
 except Exception as e:events.append({'at':time.time(),'socket_observer_error':type(e).__name__})
 (r/'run-boundary.json').write_text(json.dumps(boundary,indent=2));(r/'live-observations.json').write_text(json.dumps(events,indent=2));time.sleep(.5)
(r/'run-boundary.json').write_text(json.dumps(boundary,indent=2));(r/'live-observations.json').write_text(json.dumps(events,indent=2));print('GUARD_STOP '+str(boundary['stop_reason']),flush=True)

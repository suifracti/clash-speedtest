import importlib.util,json,subprocess,tempfile,unittest,hashlib
from pathlib import Path
SCRIPT=Path(__file__).resolve().parents[1]/'candidate_audit.py'
class CandidateAuditTests(unittest.TestCase):
 def setUp(self):
  self.tmp=tempfile.TemporaryDirectory();self.addCleanup(self.tmp.cleanup);self.repo=Path(self.tmp.name)/'repo';self.repo.mkdir()
  self.git('init','-b','main');self.git('config','user.email','audit@example.invalid');self.git('config','user.name','Synthetic audit')
  (self.repo/'feature.go').write_text('package feature\nfunc Kept() {}\n');self.git('add','.');self.git('commit','-m','base');self.base=self.git('rev-parse','HEAD');self.git('checkout','-b','candidate')
  self.spec={'features':[{'id':'kept','required':[{'path':'feature.go','contains':['func Kept(']}]}]}
 def git(self,*args):return subprocess.check_output(['git','-C',str(self.repo),*args],stderr=subprocess.DEVNULL,text=True).strip()
 def audit(self,**kwargs):
  spec=importlib.util.spec_from_file_location('candidate_audit',SCRIPT);m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
  return m.audit(self.repo,self.spec,base='main',remote_head=kwargs.pop('remote_head',self.git('rev-parse','HEAD')),**kwargs)
 def test_same_source_and_remote_is_source_only_pass(self):
  a=self.audit();self.assertTrue(a['source_ready']);self.assertEqual(a['app']['state'],'NOT_CHECKED');self.assertFalse(a['full_candidate_ready'])
 def test_remote_changed_is_not_silently_accepted(self):
  a=self.audit(remote_head='0'*40);self.assertIn('remote_head_mismatch',a['failures'])
 def test_dirty_preserved_and_rejected(self):
  p=self.repo/'feature.go';p.write_text('dirty work');before=p.read_bytes();self.assertIn('working_tree_dirty',self.audit()['failures']);self.assertEqual(p.read_bytes(),before)
 def test_lost_main_commit_is_detected(self):
  self.git('checkout','main');(self.repo/'later.go').write_text('package feature');self.git('add','.');self.git('commit','-m','later');self.git('checkout','candidate');self.assertIn('base_not_in_candidate',self.audit()['failures'])
 def test_absent_backend_not_hidden_by_frontend(self):
  self.spec['features'].append({'id':'profile','required':[{'path':'backend.go','contains':['CheckProfile(']}]});a=self.audit();self.assertEqual(a['missing_features'],['profile']);self.assertFalse(a['source_ready'])
 def test_function_removed_from_existing_file_is_detected(self):
  self.spec['features'][0]['required'][0]['contains']=['func Missing('];self.assertEqual(self.audit()['missing_features'],['kept'])
 def manifest(self):
  p=Path(self.tmp.name)/'app-core';p.write_bytes(b'synthetic executable');return p,{'source_revision':self.git('rev-parse','HEAD'),'source_tree':self.git('rev-parse','HEAD^{tree}'),'core_sha256':hashlib.sha256(p.read_bytes()).hexdigest(),'frontend_dist_sha256':'a'*64,'build_verified':True}
 def test_missing_app_manifest_rejects_full_candidate(self):
  p=self.repo/'outside-core';p.write_bytes(b'app');a=self.audit(app=p);self.assertEqual(a['app']['state'],'UNATTESTED');self.assertFalse(a['full_candidate_ready'])
 def test_wrong_app_commit_rejected(self):
  p,m=self.manifest();m['source_revision']='0'*40;a=self.audit(app=p,manifest=m);self.assertIn('app_source_revision_mismatch',a['failures'])
 def test_wrong_binary_rejected(self):
  p,m=self.manifest();p.write_bytes(b'replaced app');a=self.audit(app=p,manifest=m);self.assertIn('app_binary_hash_mismatch',a['failures'])
 def test_attested_app_does_not_count_as_runtime_acceptance(self):
  p,m=self.manifest();a=self.audit(app=p,manifest=m);self.assertTrue(a['full_candidate_ready']);self.assertEqual(a['runtime_acceptance'],'NOT_CHECKED')
 def test_guessed_manifest_is_rejected(self):
  p,m=self.manifest();m['build_verified']=False;a=self.audit(app=p,manifest=m);self.assertIn('app_build_unverified',a['failures'])
if __name__=='__main__':unittest.main()

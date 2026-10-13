import {afterEach,expect,it,vi} from 'vitest'
import {api} from '../api'
afterEach(()=>{delete window.go;vi.unstubAllGlobals()})
it('preserves typed native rejection and leaves raw storage failures unknown',async()=>{
 const start=vi.fn().mockRejectedValueOnce('CST_START_ERROR:{"admission":"not_accepted","status":400,"message":"scope unavailable"}').mockRejectedValueOnce('storage unavailable').mockRejectedValueOnce('CST_START_ERROR:invalid')
 window.go={desktop:{App:{StartWorkbenchDownloadTest:start}}};const fetch=vi.fn();vi.stubGlobal('fetch',fetch)
 await expect(api.startAttempt('download',{})).rejects.toMatchObject({status:400,admission:'not_accepted',message:'scope unavailable'})
 await expect(api.startAttempt('download',{})).rejects.toMatchObject({message:'storage unavailable'})
 await expect(api.startAttempt('download',{})).rejects.toMatchObject({message:'CST_START_ERROR:invalid'})
 expect(fetch).not.toHaveBeenCalled();expect(start).toHaveBeenCalledTimes(3)
})

it('exports frozen full scopes through Wails rather than legacy node keys',async()=>{
 const exportConfig=vi.fn().mockResolvedValue({yaml_content:'proxies: []',node_count:0})
 window.go={desktop:{App:{ExportNodesClashConfig:exportConfig}}}
 const node={profile_id:'subscription-b',node_key:'same-key',node_identity_key:'identity-b',config_revision_key:'tls-b'}
 await api.exportClash([node]);expect(exportConfig).toHaveBeenCalledWith({selections:[node]})
})

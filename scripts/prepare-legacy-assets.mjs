// The current UI is built from frontend/src. The imported legacy GUI package
// also has a go:embed directive, so its historical dependencies must exist.
// Restore only these three immutable assets, never an old source tree or patch.
import {createHash} from 'node:crypto'
import {existsSync,mkdirSync,readFileSync,writeFileSync} from 'node:fs'
import {dirname,join,resolve} from 'node:path'
import {spawnSync} from 'node:child_process'
import {fileURLToPath} from 'node:url'

const root=resolve(dirname(fileURLToPath(import.meta.url)),'..')
const baseline='219b181ef7be45e3245ece5a10d692feb53a1a2c'
const assets=[
  ['index.html',218151,'7c82e81d521a6c06a395f53136a27d2527a1fb3ae941079e58f4ebcd077337bc'],
  ['tailwind.js',407279,'176e894661aa9cdc9a5cba6c720044cbbf7b8bd80d1c9a142a7c24b1b6c50d15'],
  ['vue.global.prod.js',147534,'4963101441ded7e420c05665e7c616b2f2e3851c99e1cf8af84d29d6f10e77da'],
]
const args=process.argv.slice(2)
if(args.length&&!(args.length===2&&args[0]==='--output-dir'))throw new Error('Usage: node scripts/prepare-legacy-assets.mjs [--output-dir directory]')
const output=args.length?resolve(args[1]):join(root,'gui','web','dist')
const hash=bytes=>createHash('sha256').update(bytes).digest('hex')
mkdirSync(output,{recursive:true})
for(const [name,size,expected] of assets){
  const destination=join(output,name)
  if(existsSync(destination)){
    const existing=readFileSync(destination)
    // Existing Windows checkouts may use CRLF. Preserve equivalent files.
    if(hash(existing)!==expected&&hash(Buffer.from(existing.toString('utf8').replace(/\r\n/g,'\n')))!==expected)throw new Error(`Refusing to overwrite changed legacy asset: ${destination}`)
    continue
  }
  const result=spawnSync('git',['show',`${baseline}:gui/web/dist/${name}`],{cwd:root,maxBuffer:2*1024*1024})
  if(result.status!==0)throw new Error(`Historical asset unavailable. Fetch the existing baseline first: git fetch origin ${baseline}`)
  const bytes=result.stdout
  if(bytes.length!==size||hash(bytes)!==expected)throw new Error(`Historical asset checksum mismatch: ${name}`)
  writeFileSync(destination,bytes,{flag:'wx'})
}
console.log('Legacy embed dependencies ready; current frontend source was not changed.')

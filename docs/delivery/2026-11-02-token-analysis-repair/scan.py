
import os,json,time,hashlib,collections
root='/var/tmp/token-analysis-repair-20261102'
p='/opt/sub2api/backend/data/request-archive/2026-10-31.jsonl'
s=os.stat(p);counts=collections.Counter();total=0;last=time.monotonic();start=last
with open(p,'rb',buffering=4*1024*1024) as f,open(root+'/manifest.jsonl','w') as out:
 for num,line in enumerate(f,1):
  end=total+len(line)
  row={'line':num,'start':total,'length':len(line),'end':end,'sha256':hashlib.sha256(line).hexdigest()}
  try:
   obj=json.loads(line)
   for k in ('event','archive_id','timestamp','body_sha256','body_truncated','client_request_id','status','response_id'):
    if k in obj: row[k]=obj[k]
   counts[obj.get('event','unknown')]+=1
  except Exception as e:
   row['invalid']=type(e).__name__;counts['invalid']+=1
  out.write(json.dumps(row,ensure_ascii=True)+'\n');total=end
  if time.monotonic()-last>=20:
   print(json.dumps({'bytes':total,'total':s.st_size,'counts':counts,'elapsed':round(time.monotonic()-start)}),flush=True);last=time.monotonic()
now=os.stat(p)
assert (s.st_size,s.st_mtime_ns)==(now.st_size,now.st_mtime_ns),'source changed'
result={'path':p,'size':s.st_size,'mtime_ns':s.st_mtime_ns,'counts':counts,'elapsed':round(time.monotonic()-start)}
json.dump(result,open(root+'/scan-result.json','w'));print(json.dumps(result),flush=True)

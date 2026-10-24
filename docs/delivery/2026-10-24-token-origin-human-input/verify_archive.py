"""Read only two known archive lines; print a minimal witness to stdout.

Run on the archive host via python3 - or pass this file to python3.
No database connection, remote file writes, indexing, or service operations.
"""
import hashlib
import json
import sys

PATH = sys.argv[1] if len(sys.argv) > 1 else '/opt/sub2api/backend/data/request-archive/2026-10-24.jsonl'
ARCHIVE_ID = '73eb6d29496d6c677054f5c797996d66'
REQUEST_OFFSET = 18155124865
RESPONSE_OFFSET = 18166371053
REQUEST_SHA256 = 'a2e65ddfab9a846d1efc7c1ef6fc4f338071823aad2554d9aa938ffedcee74e1'
MAX_LINE = 2 * 1024 * 1024

def read_line(handle, offset, expected_event):
    handle.seek(offset)
    raw = handle.readline(MAX_LINE)
    if not raw.endswith(b'\n'):
        raise ValueError('Incomplete or overlong archive line at expected offset')
    event = json.loads(raw)
    if event.get('archive_id') != ARCHIVE_ID or event.get('event') != expected_event:
        raise ValueError('Archive identity does not match; stop without searching')
    return event, raw

with open(PATH, 'rb') as archive:
    request, request_raw = read_line(archive, REQUEST_OFFSET, 'request')
    response, response_raw = read_line(archive, RESPONSE_OFFSET, 'response')
if hashlib.sha256(request_raw).hexdigest() != REQUEST_SHA256:
    raise ValueError('Request line hash differs from the original evidence')

root = json.loads(request['body'])
items = root['input']
last = items[-1]
metadata = root.get('client_metadata') or {}
turn = metadata.get('x-codex-turn-metadata') or {}
if isinstance(turn, str):
    turn = json.loads(turn)
usage = response.get('usage') or {}
print(json.dumps({
    'archive_id': ARCHIVE_ID,
    'archive_path': PATH,
    'request': {
        'offset': REQUEST_OFFSET,
        'line_bytes': len(request_raw),
        'line_sha256': hashlib.sha256(request_raw).hexdigest(),
        'timestamp': request['timestamp'],
        'path': request.get('path'),
        'model': root.get('model'),
        'body_truncated': bool(request.get('body_truncated', False)),
        'input_count': len(items),
        'last_item_index': len(items) - 1,
        'last_item': {'type': last.get('type'), 'role': last.get('role'), 'content': last.get('content')},
        'metadata': {
            'x-openai-subagent': metadata.get('x-openai-subagent'),
            'subagent_kind': turn.get('subagent_kind'),
            'turn_trigger': turn.get('turn_trigger')
        }
    },
    'response': {
        'offset': RESPONSE_OFFSET,
        'line_bytes': len(response_raw),
        'line_sha256': hashlib.sha256(response_raw).hexdigest(),
        'timestamp': response['timestamp'],
        'status': response.get('status'),
        'duration_ms': response.get('duration_ms'),
        'input_tokens': usage.get('input_tokens'),
        'cached_tokens': (usage.get('input_tokens_details') or {}).get('cached_tokens'),
        'output_tokens': usage.get('output_tokens')
    }
}, ensure_ascii=False, indent=2))

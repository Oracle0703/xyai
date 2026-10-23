"""Read five previously located rows and report reminder structure in history.

Historical occurrences may be repeated across requests. No prompt text is emitted.
"""
import collections
import hashlib
import json
import re

PATH = '/opt/sub2api/backend/data/request-archive/2026-10-23.jsonl'
CASES = [
    (107745012942, 'f661db915edde612604dcd1d9de8c692a74afe2cbaf36cacd89ac0e78fc3d2d4'),
    (107794977200, '2fb4d14dfc1911ef2f4dc189590fe02ed989d4322c498d232edea6b93e6a3776'),
    (124298870556, 'e894d95bdc829cb379a8e24930e07c4f86de9808d5bb24840a1c064f07845bd8'),
    (124300277546, 'c08efd7d5e56453c607fb8f0ecdfcf6d3cedd10ea281ce3fb3d62514912b96f2'),
    (8334277225, '4b295e51d1434e7536f8a107cbe5ed2efa9208680bcdb233853c2aadb292080c'),
]
REMINDER = re.compile(r'\s*(?:<system-reminder>[\s\S]*?</system-reminder>\s*)+')
results = []
read_bytes = 0
with open(PATH, 'rb') as handle:
    for offset, expected in CASES:
        handle.seek(offset)
        raw = handle.readline(24 * 1024 * 1024)
        read_bytes += len(raw)
        assert hashlib.sha256(raw).hexdigest() == expected
        row = json.loads(raw)
        root = json.loads(row['body'])
        messages = root.get('messages', [])
        stats = collections.Counter()
        examples = []
        for index, item in enumerate(messages):
            if not isinstance(item, dict) or item.get('role') != 'user':
                continue
            content = item.get('content')
            blocks = content if isinstance(content, list) else [{'type': 'text', 'text': content}]
            texts = [block['text'] for block in blocks if isinstance(block, dict) and isinstance(block.get('text'), str)]
            contains = any('<system-reminder>' in text for text in texts)
            pure = any(REMINDER.fullmatch(text) for text in texts)
            tool = any(isinstance(block, dict) and block.get('type') == 'tool_result' for block in blocks)
            if contains:
                stats['user_text_contains_reminder'] += 1
            if pure:
                stats['user_text_pure_reminder'] += 1
            if tool and pure:
                stats['tool_and_pure_reminder'] += 1
            if contains:
                examples.append({
                    'message_index': index,
                    'previous_role': messages[index - 1].get('role') if index > 0 and isinstance(messages[index - 1], dict) else None,
                    'next_role': messages[index + 1].get('role') if index + 1 < len(messages) and isinstance(messages[index + 1], dict) else None,
                    'block_types': [block.get('type') for block in blocks if isinstance(block, dict)],
                    'text_blocks': [{'chars': len(text), 'pure_system_reminder': bool(REMINDER.fullmatch(text))} for text in texts],
                    'has_tool_result': tool,
                })
        results.append({'offset': offset, 'line_bytes_including_lf': len(raw), 'line_sha256_including_lf': expected,
                        'message_count': len(messages), 'counts': stats, 'examples': examples})
print(json.dumps({'scope': 'All historical user messages within five selected production requests. Histories overlap; occurrences are not unique requests or new turns.',
                  'read_bytes': read_bytes, 'results': results}, ensure_ascii=True, indent=2))

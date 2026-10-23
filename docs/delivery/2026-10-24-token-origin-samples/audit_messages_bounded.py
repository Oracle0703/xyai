"""Bounded, read-only structural check for Anthropic reminder/tool turns.

No prompt text or identities are exported. No Python approximation is labelled
as the Go classifier's output. This checks occurrence of structural evidence.
"""
import collections
import datetime
import hashlib
import json
import os
import sys
import time

PATH = '/opt/sub2api/backend/data/request-archive/2026-10-23.jsonl'
MAX_IO = 1024 * 1024 * 1024
WINDOW = 64 * 1024 * 1024
MAX_LINE = 24 * 1024 * 1024
MAX_SECONDS = 45
started = time.monotonic()
before = os.stat(PATH)
anchors = [(107745012942, True), (124298870556, True)] + [(int(before.st_size * i / 16), i == 0) for i in range(16)]
read_bytes = 0
counts = collections.Counter()
examples = collections.defaultdict(list)
seen = set()
windows = []


def wrappers(text):
    """Describe complete wrappers only; preserve nothing from their contents."""
    if not isinstance(text, str):
        return None
    rest = text.strip()
    tags = []
    while rest:
        for tag in ('environment_context', 'system-reminder'):
            prefix, suffix = '<' + tag + '>', '</' + tag + '>'
            if rest.startswith(prefix) and suffix in rest[len(prefix):]:
                _, rest = rest[len(prefix):].split(suffix, 1)
                rest = rest.strip()
                tags.append(tag)
                break
        else:
            return None
    return tags or None


def shape(item, index):
    result = {'index': index, 'role': item.get('role'), 'type': item.get('type')}
    content = item.get('content')
    if isinstance(content, str):
        result['content'] = {'chars': len(content), 'nonblank': bool(content.strip()), 'pure_wrappers': wrappers(content)}
    elif isinstance(content, list):
        blocks = []
        for block in content:
            if not isinstance(block, dict):
                blocks.append({'invalid': True})
                continue
            fact = {'type': block.get('type')}
            if isinstance(block.get('text'), str):
                fact.update(chars=len(block['text']), nonblank=bool(block['text'].strip()), pure_wrappers=wrappers(block['text']))
            blocks.append(fact)
        result['blocks'] = blocks
    return result


def record(kind, row, raw, offset, window, messages, index):
    counts[kind] += 1
    if len(examples[kind]) >= 4:
        return
    ua = (row.get('user_agent') or '').lower()
    examples[kind].append({
        'offset': offset, 'line_bytes_including_lf': len(raw), 'line_sha256_including_lf': hashlib.sha256(raw).hexdigest(),
        'archive_id': row.get('archive_id'), 'timestamp_utc': row.get('timestamp'), 'path': row.get('path'),
        'window': window, 'claude_cli_user_agent': ua.startswith('claude-cli/'),
        'body_truncated': row.get('body_truncated', False), 'message_count': len(messages),
        'effective_tail_index': index,
        'tail': [shape(item, i) for i, item in enumerate(messages) if i >= max(0, index - 2) and isinstance(item, dict)]})


with open(PATH, 'rb') as f:
    for wi, (anchor, aligned) in enumerate(anchors):
        if read_bytes >= MAX_IO or time.monotonic() - started >= MAX_SECONDS:
            break
        f.seek(anchor)
        align_bytes = 0
        if not aligned:
            align_bytes = len(f.readline(min(MAX_LINE, MAX_IO - read_bytes)))
            read_bytes += align_bytes
        start = f.tell()
        window = {'index': wi, 'anchor': anchor, 'known_line_boundary': aligned, 'alignment_read_bytes': align_bytes, 'start': start}
        while f.tell() - start < WINDOW and read_bytes < MAX_IO and time.monotonic() - started < MAX_SECONDS:
            offset = f.tell()
            raw = f.readline(min(MAX_LINE, WINDOW - (offset - start), MAX_IO - read_bytes))
            read_bytes += len(raw)
            if not raw or not raw.endswith(b'\n'):
                counts['window_ended_on_incomplete_read'] += 1
                break
            if offset in seen:
                counts['duplicate_line_skipped'] += 1
                continue
            seen.add(offset)
            counts['complete_lines_read'] += 1
            # Archive emits compact JSON; this prefilter is a sampling limitation.
            if b'"event":"request"' not in raw[:200] or b'/v1/messages' not in raw[:600]:
                continue
            try:
                row = json.loads(raw)
                if row.get('event') != 'request':
                    continue
                path = row.get('path') or row.get('endpoint') or ''
                if path.rstrip('/').endswith('/count_tokens'):
                    counts['count_tokens_skipped'] += 1
                    continue
                counts['messages_requests'] += 1
                if row.get('body_truncated'):
                    counts['truncated_skipped'] += 1
                    continue
                root = json.loads(row.get('body') or '')
                messages = root.get('messages')
            except (ValueError, AttributeError):
                counts['parse_error'] += 1
                continue
            if not isinstance(messages, list):
                counts['missing_messages'] += 1
                continue
            ua = (row.get('user_agent') or '').lower()
            counts['claude_cli_requests' if ua.startswith('claude-cli/') else 'other_user_agent_requests'] += 1
            idx = len(messages) - 1
            while idx >= 0 and isinstance(messages[idx], dict) and messages[idx].get('role') in ('system', 'developer'):
                idx -= 1
            if idx < 0 or not isinstance(messages[idx], dict):
                counts['no_effective_tail'] += 1
                continue
            item = messages[idx]
            if item.get('role') != 'user':
                record('effective_tail_not_user', row, raw, offset, wi, messages, idx)
                continue
            content = item.get('content')
            blocks = content if isinstance(content, list) else [{'type': 'text', 'text': content}]
            has_tool = any(isinstance(b, dict) and b.get('type') == 'tool_result' for b in blocks)
            has_reminder = any(isinstance(b, dict) and 'system-reminder' in (wrappers(b.get('text')) or []) for b in blocks)
            independent_text = any(isinstance(b, dict) and isinstance(b.get('text'), str) and b['text'].strip() and not wrappers(b['text']) for b in blocks)
            if has_tool and has_reminder:
                kind = 'tool_and_reminder_with_other_text' if independent_text else 'tool_and_reminder_without_other_text'
            elif has_reminder:
                kind = 'reminder_without_tool'
            elif has_tool:
                kind = 'tool_without_pure_reminder'
            else:
                kind = 'user_without_tool_or_pure_reminder'
            record(kind, row, raw, offset, wi, messages, idx)
        window['end'] = f.tell()
        windows.append(window)

after = os.stat(PATH)
def stat(s):
    return {'size_bytes': s.st_size, 'mtime_ns': s.st_mtime_ns, 'inode': s.st_ino}

json.dump({'collected_at_utc': datetime.datetime.now(datetime.timezone.utc).isoformat(), 'source_file': PATH,
           'source_before': stat(before), 'source_after': stat(after), 'source_stat_unchanged': stat(before) == stat(after),
           'max_io_bytes': MAX_IO, 'max_seconds': MAX_SECONDS, 'read_bytes': read_bytes,
           'duration_seconds': round(time.monotonic() - started, 3), 'counts': counts, 'windows': windows,
           'examples': examples,
           'scope': 'Targeted bounded sample. Structure only, not the Go classifier output or ground truth. Other_text is independent of complete wrappers, not proof of human authorship.'},
          sys.stdout, ensure_ascii=True, indent=2)
sys.stdout.write('\n')
if stat(before) != stat(after):
    raise SystemExit('Source stat changed; discard this evidence run.')

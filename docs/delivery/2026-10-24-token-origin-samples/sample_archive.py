"""Read-only reproduction of the 16-window sample. Writes JSON to stdout only.

No bodies, tools' arguments/results, identities, paths inside prompts, or metadata
IDs are exported. Tail entries retain their source positions and structural facts.
This is evidence collection, not a classifier or a human-labelled ground truth set.
"""
import collections
import datetime
import hashlib
import json
import os
import re
import sys

PATH = sys.argv[1] if len(sys.argv) > 1 else '/opt/sub2api/backend/data/request-archive/2026-10-23.jsonl'
LINE_LIMIT = 20 * 1024 * 1024
WINDOW_LIMIT = 32 * 1024 * 1024
TAGS = ('environment_context', 'system-reminder', 'userRequest', 'attachments',
        'context', 'editorContext', 'reminderInstructions', 'image_resize_notice', 'total_tokens')


def enum(value):
    if value is None:
        return None
    if isinstance(value, str) and re.fullmatch(r'[A-Za-z0-9_/-]{1,80}', value):
        return value
    return {'omitted_value_type': type(value).__name__}


def text_facts(value):
    if not isinstance(value, str):
        return {'value_type': type(value).__name__}
    result = {'chars': len(value), 'nonblank': bool(value.strip()), 'text_omitted': True}
    result['known_tags'] = [tag for tag in TAGS if '<' + tag + '>' in value or '<' + tag + ' ' in value]
    # Exact whole-wrapper fact, without copying its potentially private contents.
    result['environment_only'] = bool(re.fullmatch(r'\s*<environment_context>[\s\S]*?</environment_context>\s*', value))
    result['system_reminder_only'] = bool(re.fullmatch(r'\s*<system-reminder>[\s\S]*?</system-reminder>\s*', value))
    result['user_request_chars'] = [len(x) for x in re.findall(r'<userRequest>([\s\S]*?)</userRequest>', value)]
    for name, prefix in (
        ('guardian_history_delta', 'The following is the Codex agent history added since your last approval assessment.'),
        ('retained_instructions', '>>> RETAINED USER INSTRUCTIONS START'),
        ('agent_message_envelope', 'Message Type:'),
        ('translation_template', 'Translate to Simplified Chinese Language:'),
    ):
        if value.startswith(prefix):
            result['known_prefix'] = name
            break
    # Preserve only structural facts for the observed automation-looking user JSON.
    if value.lstrip().startswith('{'):
        try:
            parsed = json.loads(value)
            if isinstance(parsed, dict):
                safe_keys = ('stage', 'goal', 'package', 'page', 'sensitive', 'width', 'height', 'orientation', 'captured_at')
                result['json_known_keys'] = [key for key in safe_keys if key in parsed]
                if parsed.get('stage') == 'preflight':
                    result['json_stage'] = 'preflight'
                if parsed.get('goal') == 'Read the synthetic preflight label and return observe or wait only.':
                    result['synthetic_preflight_goal'] = True
        except (ValueError, TypeError):
            pass
    return result


def item_facts(item, index):
    if not isinstance(item, dict):
        return {'index': index, 'value_type': type(item).__name__}
    result = {'index': index, 'role': enum(item.get('role')), 'type': enum(item.get('type'))}
    content = item.get('content', item.get('parts'))
    if isinstance(content, str):
        result['content'] = text_facts(content)
    elif isinstance(content, list):
        blocks = []
        for block in content:
            if not isinstance(block, dict):
                blocks.append({'value_type': type(block).__name__})
                continue
            fact = {'type': enum(block.get('type'))}
            for key in ('functionResponse', 'function_response', 'inlineData', 'inline_data', 'fileData', 'file_data'):
                if key in block:
                    fact[key + '_present'] = True
            if 'text' in block:
                fact['text'] = text_facts(block['text'])
            blocks.append(fact)
        result['content_blocks'] = blocks
    else:
        result['content_value_type'] = type(content).__name__
    return result


def snapshot():
    st = os.stat(PATH)
    return {'size_bytes': st.st_size, 'mtime_ns': st.st_mtime_ns, 'inode': st.st_ino}


before = snapshot()
samples, windows = [], []
counts, endpoints, metadata_combinations = collections.Counter(), collections.Counter(), collections.Counter()
read_bytes, alignment_bytes = 0, 0
with open(PATH, 'rb') as archive:
    for anchor in range(16):
        position = int(before['size_bytes'] * anchor / 16)
        archive.seek(position)
        alignment = len(archive.readline(LINE_LIMIT)) if position else 0
        alignment_bytes += alignment
        start = archive.tell()
        window = {'index': anchor, 'anchor_offset': position, 'alignment_discard_bytes': alignment, 'start_offset': start, 'requests': 0}
        while archive.tell() - start < WINDOW_LIMIT and window['requests'] < 20:
            offset = archive.tell()
            line = archive.readline(LINE_LIMIT)
            read_bytes += len(line)
            if not line:
                window['stop'] = 'eof'
                break
            if not line.endswith(b'\n'):
                counts['overlong_or_partial'] += 1
                window['stop'] = 'line_limit_or_incomplete'
                window['stopped_line_offset'] = offset
                window['stopped_read_bytes'] = len(line)
                break
            try:
                row = json.loads(line)
            except ValueError:
                counts['invalid_envelope'] += 1
                continue
            if row.get('event') != 'request':
                continue
            window['requests'] += 1
            endpoints[row.get('endpoint', '')] += 1
            sample = {'sample': len(samples) + 1, 'window': anchor, 'offset': offset, 'line_bytes_including_lf': len(line),
                      'line_sha256_including_lf': hashlib.sha256(line).hexdigest(), 'archive_id': row.get('archive_id'),
                      'timestamp_utc': row.get('timestamp'), 'endpoint': row.get('endpoint'),
                      'path': row.get('path'), 'body_truncated': row.get('body_truncated', False)}
            samples.append(sample)
            try:
                body = json.loads(row.get('body', ''))
            except ValueError:
                counts['invalid_body'] += 1
                sample['body_parse'] = 'invalid_json'
                continue
            if not isinstance(body, dict):
                counts['nonobject'] += 1
                sample['body_parse'] = 'nonobject'
                continue
            md = body.get('client_metadata') or {}
            turn = md.get('x-codex-turn-metadata') or {}
            sample['turn_metadata_encoding'] = type(turn).__name__
            if isinstance(turn, str):
                try:
                    turn = json.loads(turn)
                except ValueError:
                    turn = {}
            if not isinstance(turn, dict):
                turn = {}
            sample['metadata'] = {'x-openai-subagent': enum(md.get('x-openai-subagent'))}
            sample['metadata'].update({key: enum(turn.get(key)) for key in ('request_kind', 'turn_trigger', 'thread_source', 'subagent_kind')})
            seq = body.get('input', body.get('messages', body.get('contents', [])))
            sample['input_container'] = 'input' if 'input' in body else 'messages' if 'messages' in body else 'contents' if 'contents' in body else None
            if isinstance(seq, str):
                sample['input_string'] = text_facts(seq)
                seq = [{'role': 'user', 'content': seq}]
            if not isinstance(seq, list):
                seq = []
            sample['item_count'] = len(seq)
            sample['tail'] = [item_facts(item, index) for index, item in enumerate(seq) if index >= len(seq) - 6]
            tail = seq[-1] if seq and isinstance(seq[-1], dict) else {}
            content = tail.get('content', tail.get('parts', []))
            types = [c.get('type') or '|'.join(c) for c in content if isinstance(c, dict)] if isinstance(content, list) else ['string']
            signature = str(row.get('endpoint', '')) + ' / ' + str(tail.get('role', '')) + ' / ' + str(tail.get('type', '')) + ' / ' + ','.join(types)
            sample['terminal_signature'] = signature
            counts[signature] += 1
            metadata_combinations[json.dumps([md.get('x-openai-subagent'), turn.get('request_kind'), turn.get('turn_trigger'), turn.get('thread_source'), tail.get('type'), tail.get('role')])] += 1
        window.setdefault('stop', 'request_limit' if window['requests'] >= 20 else 'byte_budget')
        window['end_offset'] = archive.tell()
        windows.append(window)

after = snapshot()
output = {'schema_version': 1, 'collected_at_utc': datetime.datetime.now(datetime.timezone.utc).isoformat(),
          'source_file': PATH, 'source_before': before, 'source_after': after, 'source_stat_unchanged': before == after,
          'sampling': {'windows': 16, 'requests_per_window_max': 20, 'window_budget_bytes': WINDOW_LIMIT, 'readline_limit_bytes': LINE_LIMIT,
                       'note': 'Byte budget is tested before each read and may overshoot by one line; initial alignment discards are excluded from legacy_read_bytes.'},
          'legacy_read_bytes': read_bytes, 'alignment_discard_bytes': alignment_bytes, 'actual_read_bytes': read_bytes + alignment_bytes,
          'requests_sampled': len(samples), 'endpoint_counts': endpoints, 'terminal_counts': counts,
          'metadata_combinations': metadata_combinations, 'windows': windows, 'samples': samples,
          'redaction': 'All text contents and metadata IDs omitted. Structural facts only; not raw requests, not independently labelled ground truth.'}
json.dump(output, sys.stdout, ensure_ascii=True, indent=2)
sys.stdout.write('\n')
if before != after:
    raise SystemExit('Source stat changed during sampling; reject this evidence run.')

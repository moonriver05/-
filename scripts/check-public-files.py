"""Lightweight tracked-file publishing guard, not a full history audit.

The PowerShell check additionally compares local configured credentials.
Never print matched credential values.
"""
import re
import subprocess
import sys
from pathlib import Path

root = Path(__file__).resolve().parents[1]
files = subprocess.check_output(['git', 'ls-files', '-z'], cwd=root).decode('utf-8').split('\0')
patterns = [re.compile(p) for p in [
    r'-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----',
    r'\bgh[pousr]_[A-Za-z0-9]{30,}', r'\bgithub_pat_[A-Za-z0-9_]{40,}',
    r'\bsk-(?:proj-)?[A-Za-z0-9_-]{24,}',
]]
issues = []
for name in filter(None, files):
    path = root / name
    if path.name.startswith('.env') and path.name != '.env.example':
        issues.append(name + ': private environment file')
        continue
    if not path.is_file() or path.stat().st_size > 2_000_000:
        continue  # Public data shards have separate checksum verification.
    try:
        text = path.read_text(encoding='utf-8')
    except UnicodeError:
        continue
    if any(pattern.search(text) for pattern in patterns):
        issues.append(name + ': credential pattern detected')
if issues:
    print('\n'.join(issues))
    sys.exit(1)
print('Tracked public-file check passed; values never printed.')

"""Run the bounded RAG indexer with server-side .env (Python 3.6+)."""
import os
import re
import sys

root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
env = os.environ.copy()
with open(os.path.join(root, '.env'), encoding='utf-8-sig') as source:
    for line in source:
        line = line.strip()
        if not line or line.startswith('#') or '=' not in line:
            continue
        name, value = line.split('=', 1)
        name, value = name.strip(), value.strip()
        if not re.match(r'^[A-Za-z_][A-Za-z0-9_]*$', name):
            continue
        if len(value) >= 2 and value[0] == value[-1] and value[0] in ('"', "'"):
            value = value[1:-1]
        env[name] = value
binary = os.path.join(root, 'backend', 'bin', 'mathwiki-rag-indexer')
os.chdir(os.path.join(root, 'backend'))
os.execve(binary, [binary] + sys.argv[1:], env)

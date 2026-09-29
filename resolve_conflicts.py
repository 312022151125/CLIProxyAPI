import re, sys

for filepath in sys.argv[1:]:
    with open(filepath, 'r') as f:
        content = f.read()
    pattern = r'<<<<<<< HEAD\n.*?=======\n(.*?)>>>>>>> upstream/main\n'
    resolved = re.sub(pattern, r'\1', content, flags=re.DOTALL)
    with open(filepath, 'w') as f:
        f.write(resolved)
    print(f"Resolved {filepath}, conflicts remaining: {resolved.count('<<<<<<<')}")

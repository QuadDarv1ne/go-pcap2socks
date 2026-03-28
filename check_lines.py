with open('main.go', 'rb') as f:
    c = f.read()

# Try both line endings
if b'\r\n' in c:
    lines = c.split(b'\r\n')
    print('Using CRLF')
else:
    lines = c.split(b'\n')
    print('Using LF')

print('Total lines:', len(lines))
print('Line 769:', repr(lines[768][:100]))
print('Line 770:', repr(lines[769][:100]))
print('Line 771:', repr(lines[770][:100]))
print('Line 771:', repr(lines[770]))
print('Line 772:', repr(lines[771][:100]))

# Check for broken lines
for i, line in enumerate(lines):
    if b'localho' in line and not b'localhost' in line:
        print(f'Found broken localho at line {i+1}: {repr(line)}')

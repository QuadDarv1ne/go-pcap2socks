with open('main.go', 'rb') as f:
    c = f.read()

lines = c.split(b'\r\n')
print('Total lines:', len(lines))
for i in range(max(0, len(lines)-10), len(lines)):
    print(f'{i+1}: {repr(lines[i][:80])}')

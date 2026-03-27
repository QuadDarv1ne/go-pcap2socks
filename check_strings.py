with open('main.go', 'rb') as f:
    c = f.read()

# Find all string literals with newlines
in_string = False
string_start = 0
escape = False

for i in range(len(c)):
    byte = c[i:i+1]
    
    if byte == b'"' and not escape:
        if not in_string:
            in_string = True
            string_start = i
        else:
            in_string = False
    elif byte == b'\\' and not escape:
        escape = True
    else:
        escape = False
    
    if in_string and byte == b'\n':
        context_start = max(0, string_start - 20)
        context_end = min(len(c), i + 20)
        print(f'Newline in string at {i}')
        print(f'Context: {repr(c[context_start:context_end])}')
        print('---')

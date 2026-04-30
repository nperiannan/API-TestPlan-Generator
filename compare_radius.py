import yaml

def get_cases(path):
    d = yaml.safe_load(open(path, encoding='utf-8'))
    cases = []
    for f in d.get('features', []):
        tests = f.get('tests') or f.get('testCases') or {}
        for cat, tcs in tests.items():
            if isinstance(tcs, list):
                for tc in tcs:
                    desc = tc.get('description', '') or ''
                    cases.append({
                        'id':   tc.get('testCaseID', ''),
                        'type': tc.get('type', ''),
                        'desc': desc,
                        'priority': tc.get('priority', ''),
                    })
    return cases

old_list = get_cases('radius-server.yaml')
new_list = get_cases('Testplans/radius-server.yaml')

old_descs = {tc['desc'] for tc in old_list}
new_descs = {tc['desc'] for tc in new_list}

added   = [tc for tc in new_list if tc['desc'] not in old_descs]
removed = [tc for tc in old_list if tc['desc'] not in new_descs]

print("Old count:", len(old_list), "  New count:", len(new_list))
print()
print("New test cases added (" + str(len(added)) + "):")
for tc in added:
    print("  " + tc['id'] + " [" + tc['type'] + "] " + tc['priority'] + " - " + tc['desc'])
print()
print("Test cases removed (" + str(len(removed)) + "):")
for tc in removed:
    print("  " + tc['id'] + " [" + tc['type'] + "] " + tc['priority'] + " - " + tc['desc'])

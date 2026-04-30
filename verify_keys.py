import yaml

with open('Testplans/radius-server.yaml', 'r') as f:
    data = yaml.safe_load(f)

# Check specific test cases
target_ids = ['TCXM_5669']  # create performance

for feat in data['features']:
    for cat, tests in feat['tests'].items():
        for tc in tests:
            if tc['testCaseID'] in target_ids:
                print(f'\n=== {tc["testCaseID"]}: {tc["description"][:80]} ===')
                for i, step in enumerate(tc['steps']):
                    method = step.get('method', '')
                    name = step.get('name', '')
                    body = step.get('body', {})
                    objs = body.get('objects', [])
                    props_str = ''
                    if objs:
                        for prop in objs[0].get('properties', []):
                            props_str += f' {prop["name"]}={prop["value"]}'
                    if props_str:
                        print(f'  Step {i}: {method} {name} |{props_str}')

# Now check ALL tests for duplicate server IPs in create operations
print('\n\n=== Checking ALL radius-server tests for duplicate key IPs in CREATE ops ===')
found = False
for feat in data['features']:
    for cat, tests in feat['tests'].items():
        for tc in tests:
            create_ips = []
            for step in tc['steps']:
                body = step.get('body', {})
                objs = body.get('objects', [])
                op = body.get('operation', '')
                if objs and op == 'add':
                    for prop in objs[0].get('properties', []):
                        if prop['name'] == 'server':
                            create_ips.append(prop['value'])
            if len(create_ips) > 1 and len(set(create_ips)) < len(create_ips):
                found = True
                print(f'[{cat}] DUPLICATE: {tc["testCaseID"]} - IPs: {create_ips}')
if not found:
    print('No duplicate server IPs found in any create operations!')

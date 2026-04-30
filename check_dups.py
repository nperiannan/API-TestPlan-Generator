import yaml

with open('Testplans/radius-server.yaml', 'r') as f:
    data = yaml.safe_load(f)

for feat in data['features']:
    for cat, tests in feat['tests'].items():
        for tc in tests:
            # Count only CREATE steps (POST with operation=add) - not updates
            create_ips = []
            for step in tc['steps']:
                method = step.get('method', '')
                body = step.get('body', {})
                objs = body.get('objects', [])
                op = body.get('operation', '')
                if objs and method == 'POST' and op == 'add':
                    for prop in objs[0].get('properties', []):
                        if prop['name'] == 'server':
                            create_ips.append(prop['value'])
            if len(create_ips) > 1 and len(set(create_ips)) < len(create_ips):
                print(f'[{cat}] DUPLICATE CREATE IPs: {tc["testCaseID"]} - {tc["description"][:80]}')
                print(f'  Create IPs: {create_ips}')

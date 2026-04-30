import yaml

with open('Testplans/radius-server.yaml', 'r') as f:
    data = yaml.safe_load(f)

for feat in data['features']:
    for cat, tests in feat['tests'].items():
        for tc in tests:
            if tc['testCaseID'] in ['TCXM_5571', 'TCXM_5572', 'TCXM_5573', 'TCXM_5574']:
                print(f'\n=== {tc["testCaseID"]}: {tc["description"][:80]} ===')
                for i, step in enumerate(tc['steps']):
                    method = step.get('method', '')
                    name = step.get('name', '')
                    body = step.get('body', {})
                    op = body.get('operation', '')
                    server = ''
                    priority = ''
                    objs = body.get('objects', [])
                    if objs:
                        for prop in objs[0].get('properties', []):
                            if prop['name'] == 'server':
                                server = prop['value']
                            if prop['name'] == 'priority':
                                priority = prop['value']
                    print(f'  Step {i}: {method} {name} | op={op} server={server} priority={priority}')

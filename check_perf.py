import yaml

with open('Testplans/radius-server.yaml', 'r') as f:
    data = yaml.safe_load(f)

target_ids = ['TCXM_5667', 'TCXM_5671']

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
                    server = ''
                    if objs:
                        for prop in objs[0].get('properties', []):
                            if prop['name'] == 'server':
                                server = prop['value']
                    if server:
                        print(f'  Step {i}: {method} {name} | server={server}')

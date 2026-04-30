import yaml

for fname, expected_key in [
    ('Testplans/dns-server.yaml', 'server'),
    ('Testplans/ntp-server.yaml', 'server'),
    ('Testplans/syslog-server.yaml', 'server'),
]:
    with open(fname, 'r') as f:
        data = yaml.safe_load(f)
    
    feature_name = fname.split('/')[-1].replace('.yaml', '')
    print(f'\n=== {feature_name} ===')
    
    for feat in data['features']:
        for cat, tests in feat['tests'].items():
            for tc in tests:
                if 'create' in tc.get('description', '').lower() and ('instance' in tc.get('description', '').lower() or 'performance' in tc.get('description', '').lower() or 'capacity' in tc.get('description', '').lower()):
                    servers = []
                    for step in tc['steps']:
                        body = step.get('body', {})
                        objs = body.get('objects', [])
                        if objs:
                            for prop in objs[0].get('properties', []):
                                if prop['name'] == expected_key:
                                    servers.append(prop['value'])
                    if servers:
                        unique_pct = len(set(servers)) / len(servers) * 100 if servers else 0
                        status = 'OK' if len(set(servers)) == len(servers) else 'DUPS!'
                        print(f'  [{cat}] {tc["testCaseID"]}: {len(servers)} creates, {len(set(servers))} unique [{status}]')
                        if len(servers) <= 15:
                            print(f'    Values: {servers}')

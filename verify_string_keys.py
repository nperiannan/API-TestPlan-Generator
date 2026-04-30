import yaml

for fname, key_field in [
    ('Testplans/static-route.yaml', 'route-name'),
    ('Testplans/wlan.yaml', 'ssid'),
    ('Testplans/snmp-global-config.yaml', 'community'),
]:
    with open(fname, 'r') as f:
        data = yaml.safe_load(f)
    
    feature_name = fname.split('/')[-1].replace('.yaml', '')
    print(f'\n=== {feature_name} (key: {key_field}) ===')
    
    for feat in data['features']:
        for cat, tests in feat['tests'].items():
            for tc in tests:
                desc = tc.get('description', '').lower()
                if ('create' in desc and ('instance' in desc or 'performance' in desc or 'capacity' in desc)):
                    vals = []
                    for step in tc['steps']:
                        body = step.get('body', {})
                        objs = body.get('objects', [])
                        if objs:
                            for prop in objs[0].get('properties', []):
                                if prop['name'] == key_field:
                                    vals.append(prop['value'])
                        elif key_field in body:
                            vals.append(body[key_field])
                    if vals:
                        unique_pct = len(set(vals)) / len(vals) * 100 if vals else 0
                        status = 'OK' if len(set(vals)) == len(vals) else 'DUPS!'
                        print(f'  [{cat}] {tc["testCaseID"]}: {len(vals)} creates, {len(set(vals))} unique [{status}]')
                        if len(vals) <= 12:
                            print(f'    Values: {vals}')

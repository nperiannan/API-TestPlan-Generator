import yaml, glob

for fname in glob.glob('Testplans/*.yaml'):
    with open(fname, 'r') as f:
        try:
            data = yaml.safe_load(f)
        except:
            continue
    if not data or 'features' not in data:
        continue
    feature_name = data['features'][0].get('name', data['features'][0].get('feature', '?')) if data['features'] else '?'
    for feat in data['features']:
        for cat, tests in feat['tests'].items():
            for tc in tests:
                # Look for multi-create tests (scale, perf create)
                create_key_vals = {}
                for step in tc['steps']:
                    body = step.get('body', {})
                    objs = body.get('objects', [])
                    op = body.get('operation', '')
                    method = step.get('method', '')
                    if not objs or op != 'add' or method != 'POST':
                        continue
                    # Collect all property values for this create step
                    props = {p['name']: p['value'] for p in objs[0].get('properties', [])}
                    key = str(props)
                    if key in create_key_vals:
                        create_key_vals[key] += 1
                    else:
                        create_key_vals[key] = 1
                # Check if any full property set is duplicated
                dups = {k: v for k, v in create_key_vals.items() if v > 1}
                if dups:
                    print(f'[{feature_name}/{cat}] {tc["testCaseID"]}: {len(dups)} duplicated create payloads')
                    for k, v in dups.items():
                        print(f'  {v}x: {k[:120]}')

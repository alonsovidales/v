import sys
import collections
import json

MAX_OPS_TO_CONSIDERER_ACTION = 5
SECS_TO_STUDY = 1800

curr_prices = collections.defaultdict(list)
print("Parsing file...")
with open(sys.argv[1], 'r') as f:
    for line in f:
        parts = line.split(':', 1)
        try:
            curr_prices[parts[0]].append(json.loads(parts[1]))
        except:
            print('Line ignored:', parts)

def ts_to_sec(ts):
    return ts / 1000000000

print("File parsed...")

total_none = 0
total_buy = 0
total_sell = 0
for c, v in curr_prices.items():
    c = 'USD'
    v = curr_prices['USD']
    with open('training_set_%s.json' % c, 'w') as out_f:
        print("Using currency:", c)
        end = 0
        for i in xrange(len(v)):
            new_range = False
            if i % 100 == 0:
                print("Progress: %f" % round((float(i) / len(v)) * 100, 2))

            while ts_to_sec(v[end]['t']) - ts_to_sec(v[i]['t']) < SECS_TO_STUDY and end < len(v)-1:
                new_range = True
                end += 1

            if new_range and end < len(v)-1:
                # Now we check if there is something good in the next hour
                future_end = end
                # We use end + 1 because is closest to the actual value we are
                # going to get caused by the delay on placing the operation
                max_bid = v[end+1]['b']
                min_ask = v[end+1]['a']
                total_ask_increases = 0
                total_bid_increases = 0
                while ts_to_sec(v[future_end]['t']) - ts_to_sec(v[end]['t']) < (SECS_TO_STUDY/2) and future_end < len(v)-1:
                    future_end += 1
                    if v[future_end]['b'] > max_bid:
                        max_bid = v[future_end]['b']
                        total_bid_increases += 1
                    if v[future_end]['a'] < min_ask:
                        min_ask = v[future_end]['a']
                        total_ask_increases += 1

                # [Nothing, Ask, Buy]
                action = [1, 0, 0]
                if max_bid > v[end]['a'] and total_ask_increases > MAX_OPS_TO_CONSIDERER_ACTION:
                    action[0] = 0
                    action[1] = 1
                    total_buy += 1
                elif min_ask < v[end]['b'] and total_bid_increases > MAX_OPS_TO_CONSIDERER_ACTION:
                    action[0] = 0
                    action[2] = 1
                    total_sell += 1
                else:
                    total_none += 1

                hour_group = collections.defaultdict(list)
                for point in v[i:end]:
                    hour_group[ts_to_sec(point['t'])].append(point)

                def get_avg_vals(list_vals):
                    avg_ask = 0.
                    avg_bid = 0.
                    for v in list_vals:
                        avg_ask += v['a']
                        avg_bid += v['b']

                    return avg_ask/len(list_vals),  avg_bid/len(list_vals)

                total_avg_a, total_avg_b = get_avg_vals(v[i:end])
                current_val_a, current_val_b = get_avg_vals(hour_group[ts_to_sec(v[i]['t'])])
                prev_val_a = current_val_a
                prev_val_b = current_val_b
                result = []
                for sec in xrange(ts_to_sec(v[i]['t']), ts_to_sec(v[end]['t'])):
                    if sec in hour_group:
                        prev_val_a = current_val_a
                        prev_val_b = current_val_b
                        current_val_a, current_val_b = get_avg_vals(hour_group[sec])

                        result.append([
                            current_val_a/prev_val_a,
                            current_val_b/prev_val_b,
                            current_val_a/current_val_b,
                            current_val_a/total_avg_a,
                            current_val_b/total_avg_b
                        ])
                    else:
                        result.append([1., 1., 1., 1., 1.])

                out_f.write("{}:{}\n".format(json.dumps(result[-SECS_TO_STUDY:]), json.dumps(action)))
                #out_f.write("{}:{}\n".format(json.dumps(points_bid), json.dumps(action)))
                #print(c, len(v[i:end]), i, end, ts_to_sec(v[i]['t']), ts_to_sec(v[end]['t']) - ts_to_sec(v[i]['t'])), action

        print("Sell: %d, Buy: %d, None: %d" % (total_sell, total_buy, total_none))
        exit()

#!/usr/bin/env python3
"""Apply knockout stage labels to the seed matches JSON file.

Reads football.matches.json, adds home_team_label and away_team_label
for all knockout matches based on the official 2026 World Cup bracket,
and fixes known team ID mismatches.
"""
import json
import os
import sys

SEED_FILE = os.path.join(os.path.dirname(__file__), "..", "data", "seed", "football.matches.json")

R32_LABELS = {
    73: {"home": "Runner-up Group A", "away": "Runner-up Group B"},
    74: {"home": "Winner Group E", "away": "Runner-up Group D"},
    75: {"home": "Winner Group F", "away": "Runner-up Group C"},
    76: {"home": "Winner Group C", "away": "Runner-up Group F"},
    77: {"home": "Winner Group I", "away": "3rd Group F"},
    78: {"home": "Runner-up Group E", "away": "3rd Group I"},
    79: {"home": "Winner Group A", "away": "3rd Group E"},
    80: {"home": "Winner Group L", "away": "3rd Group E/H/I/J/K"},
    81: {"home": "Winner Group D", "away": "3rd Group B"},
    82: {"home": "Winner Group G", "away": "3rd Group A/E/H/I/J"},
    83: {"home": "Runner-up Group K", "away": "Runner-up Group L"},
    84: {"home": "Winner Group H", "away": "Runner-up Group J"},
    85: {"home": "Winner Group B", "away": "3rd Group E/F/G/I/J"},
    86: {"home": "Winner Group J", "away": "Runner-up Group H"},
    87: {"home": "Winner Group K", "away": "3rd Group L"},
    88: {"home": "Runner-up Group D", "away": "Runner-up Group G"},
}

LATER_LABELS = {
    89:  {"home": "Winner Match 74", "away": "Winner Match 77"},
    90:  {"home": "Winner Match 73", "away": "Winner Match 75"},
    91:  {"home": "Winner Match 76", "away": "Winner Match 78"},
    92:  {"home": "Winner Match 79", "away": "Winner Match 80"},
    93:  {"home": "Winner Match 83", "away": "Winner Match 84"},
    94:  {"home": "Winner Match 81", "away": "Winner Match 82"},
    95:  {"home": "Winner Match 86", "away": "Winner Match 88"},
    96:  {"home": "Winner Match 85", "away": "Winner Match 87"},
    97:  {"home": "Winner Match 89", "away": "Winner Match 90"},
    98:  {"home": "Winner Match 93", "away": "Winner Match 94"},
    99:  {"home": "Winner Match 91", "away": "Winner Match 92"},
    100: {"home": "Winner Match 95", "away": "Winner Match 96"},
    101: {"home": "Winner Match 97", "away": "Winner Match 98"},
    102: {"home": "Winner Match 99", "away": "Winner Match 100"},
    103: {"home": "", "away": ""},
    104: {"home": "Winner Match 101", "away": "Winner Match 102"},
}

ALL_LABELS = {}
ALL_LABELS.update(R32_LABELS)
ALL_LABELS.update(LATER_LABELS)

# Fix team ID mismatches in seed data
TEAM_ID_FIXES = {
    80: {"home_team_id": "45"},  # Belgium(25) -> England(45) - Match 80 is England
    77: {"away_team_id": "23"},  # TBD(0) -> Sweden(23) - Match 77 is France vs Sweden
    79: {"away_team_id": "20"},  # TBD(0) -> Ecuador(20) - Match 79 is Mexico vs Ecuador
}

# Group matches with null scores that need results for standings computation
GROUP_SCORE_FIXES = {
    67: {"home_score": "0", "away_score": "2", "finished": "TRUE", "time_elapsed": "finished"},
    68: {"home_score": "2", "away_score": "1", "finished": "TRUE", "time_elapsed": "finished"},
    69: {"home_score": "1", "away_score": "1", "finished": "TRUE", "time_elapsed": "finished"},
    70: {"home_score": "0", "away_score": "3", "finished": "TRUE", "time_elapsed": "finished"},
    71: {"home_score": "2", "away_score": "1", "finished": "TRUE", "time_elapsed": "finished"},
    72: {"home_score": "0", "away_score": "0", "finished": "TRUE", "time_elapsed": "finished"},
}


def main():
    with open(SEED_FILE, "r") as f:
        matches = json.load(f)

    changes = 0
    for m in matches:
        mid = int(m["id"])

        # Fix team IDs
        if mid in TEAM_ID_FIXES:
            for key, val in TEAM_ID_FIXES[mid].items():
                if m.get(key) != val:
                    m[key] = val
                    changes += 1

        # Fix group scores
        if mid in GROUP_SCORE_FIXES:
            for key, val in GROUP_SCORE_FIXES[mid].items():
                if m.get(key) != val:
                    m[key] = val
                    changes += 1

        # Apply labels
        if mid in ALL_LABELS:
            labels = ALL_LABELS[mid]
            if m.get("home_team_label") != labels["home"] or m.get("away_team_label") != labels["away"]:
                m["home_team_label"] = labels["home"]
                m["away_team_label"] = labels["away"]
                changes += 1

    with open(SEED_FILE, "w") as f:
        json.dump(matches, f, indent=2, ensure_ascii=False)

    print(f"Applied {changes} changes to {SEED_FILE}")
    return 0 if changes >= 0 else 1


if __name__ == "__main__":
    sys.exit(main())

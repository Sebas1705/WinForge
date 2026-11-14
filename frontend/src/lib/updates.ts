import type {UpgradeInfo} from "./model";

/** Splits the list of available updates into the ones that count and the ones the person chose to hide. */
export function splitUpgrades(list: UpgradeInfo[]): { active: UpgradeInfo[]; ignored: UpgradeInfo[] } {
    const active: UpgradeInfo[] = [];
    const ignored: UpgradeInfo[] = [];
    for (const u of list) (u.ignore ? ignored : active).push(u);
    return {active, ignored};
}

/** The updates that count, or null while nothing has been checked yet. */
export function pendingUpgrades(list: UpgradeInfo[] | null): UpgradeInfo[] | null {
    return list === null ? null : splitUpgrades(list).active;
}

/** The pause lengths offered, in days. The backend accepts 1 to 365. */
export const PAUSE_DAYS = [7, 30, 90] as const;

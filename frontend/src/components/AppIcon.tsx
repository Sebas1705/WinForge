import {useContext, useState} from "react";
import {IconsContext, iconUrl} from "../lib/icons";
import {avatarHue, initials} from "../lib/tally";

/**
 * The app's own icon on a light tile (so dark glyphs stay legible in the dark
 * theme), or colored initials when no icon is known or it fails to load.
 */
export function AppIcon({id, name, category, size = 34}: { id: string; name: string; category: string; size?: number }) {
    const index = useContext(IconsContext);
    const [broken, setBroken] = useState(false);
    const url = iconUrl(index, id);
    const style = {width: size, height: size, ["--size" as string]: `${size}px`, ["--hue" as string]: avatarHue(category)};
    if (!url || broken) {
        return <span className="avatar" style={{...style, fontSize: Math.max(9, size * 0.34)}} aria-hidden>{initials(name)}</span>;
    }
    return (
        <span className="appicon" style={style} aria-hidden>
            <img src={url} alt="" loading="lazy" decoding="async" onError={() => setBroken(true)}/>
        </span>
    );
}

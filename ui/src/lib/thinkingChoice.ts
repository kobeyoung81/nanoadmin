// The thinking choices a mode may store. `''` means "follow the type/platform
// default", which is different from an explicit "off".

import type { ThinkingLevel } from './thinking'

export type ThinkingChoice = ThinkingLevel | ''

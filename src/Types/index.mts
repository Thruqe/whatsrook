export * from "./Auth.mts";
export * from "./BinaryNode.mts";
export * from "./GroupMetadata.mts";
export * from "./Chat.mts";
export * from "./Contact.mts";
export * from "./Reachout.mts";
export * from "./State.mts";
export * from "./Message.mts";
export * from "./Socket.mts";
export * from "./Events.mts";
export * from "./Call.mts";
export * from "./Newsletter.mts";

import type { AuthenticationState } from "./Auth.mts";
import type { SocketConfig } from "./Socket.mts";

export type UserFacingSocketConfig = Partial<SocketConfig> & {
	auth: AuthenticationState;
};

export type BrowsersMap = {
	ubuntu(browser: string): [string, string, string];
	macOS(browser: string): [string, string, string];
	baileys(browser: string): [string, string, string];
	windows(browser: string): [string, string, string];
	android(browser: string): [string, string, string];
	appropriate(browser: string): [string, string, string];
};

export enum DisconnectReason {
	connectionClosed = 428,
	timedOut = 408,
	connectionReplaced = 440,
	loggedOut = 401,
	badSession = 500,
	restartRequired = 515,
	forbidden = 403,
	unavailableService = 503
}

export type WAInitResponse = {
	ref: string;
	ttl: number;
	status: 200;
};

export type WABusinessHoursConfig = {
	day_of_week: string;
	mode: string;
	open_time?: number;
	close_time?: number;
};

export type WABusinessProfile = {
	description: string;
	email: string | undefined;
	business_hours: {
		timezone?: string;
		config?: WABusinessHoursConfig[];
		business_config?: WABusinessHoursConfig[];
	};
	website: string[];
	category?: string;
	wid?: string;
	address?: string;
};

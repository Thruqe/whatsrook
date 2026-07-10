import P from "pino";

export const logger = P({
  level: "trace",
  transport: {
    targets: [
      {
        target: "pino-pretty",
        options: { colorize: true },
        level: "trace",
      },
      {
        target: "pino/file",
        options: { destination: "./logs.txt" },
        level: "trace",
      },
    ],
  },
});

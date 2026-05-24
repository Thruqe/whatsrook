use std::env;

pub struct CliArgs {
    pub session: String,
    pub pair: Option<String>,
    pub qrcode: bool,
    pub logout: bool,
}

impl CliArgs {
    pub fn parse() -> Self {
        let args: Vec<String> = env::args().collect();

        if args.contains(&"-h".to_string()) || args.contains(&"--help".to_string()) {
            println!("Usage: zevBot --session <phone_number> [OPTIONS]

Options:
  --session <phone>   Phone number used to identify the session (required)
  --pair <phone>      Request a pair code for the given phone number
  --qrcode            Print the QR code to stdout for scanning
  --logout            Remove the session auth files and exit
  -h, --help          Show this help message");
            std::process::exit(0);
        }

        let get_value = |flag: &str| -> Option<String> {
            let index = args.iter().position(|a| a == flag)?;
            args.get(index + 1).cloned()
        };

        let session = get_value("--session").unwrap_or_else(|| {
            eprintln!("Error: --session <phone_number> is required. Run with -h for help.");
            std::process::exit(1);
        });

        Self {
            session,
            pair: get_value("--pair"),
            qrcode: args.contains(&"--qrcode".to_string()),
            logout: args.contains(&"--logout".to_string()),
        }
    }
}
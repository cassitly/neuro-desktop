mod os_agent;
use os_agent::OsAgent;

fn main() {
    let mut agent = OsAgent::start();

    agent.move_mouse(400, 300);
    agent.click(400, 300);
    agent.type_text("Hello from Neuro 👋");
}
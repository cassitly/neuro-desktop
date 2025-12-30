mod controller;
use controller::Controller;

mod integration;
use integration::{start_integration};

#[tokio::main]
async fn main() {
    let controller = Controller::initialize_drivers().expect("Failed to start Controller Drivers");

    start_integration(controller);

    // controller.mouse_move(400, 300).expect("Failed to move mouse");
    // controller.mouse_click(400, 300).expect("Failed to click");
    // controller.type_text("Hello from Neuro 👋").expect("Failed to type text");

    // println!("{}", controller.action_history().expect("Failed to get action history"));

    // controller.execute_instructions().expect("Failed to execute instructions");

    // Ok(())

    // let (neuro_tx, mut neuro_rx) = start_integration(
    //     "Neuro's Desktop",
    //     "ws://localhost:8080/" | "ws://localhost:1337",
    // )
    // .await;

    // // Send initial context
    // neuro_tx
    //     .send(NeuroInput::Context(
    //         "Initial context".into(),
    //     ))
    //     .await
    //     .unwrap();

    // // Game loop
    // loop {
    //     if let Some(action) = neuro_rx.recv().await {
    //         println!("Neuro chose: {}", action.action);

    //         // YOU handle what this does
    //         // ...

    //         // Report result
    //         neuro_tx
    //             .send(NeuroInput::ActionResult {
    //                 action: action.action,
    //                 result: "success".into(),
    //             })
    //             .await
    //             .unwrap();
    //     }
    // }
}
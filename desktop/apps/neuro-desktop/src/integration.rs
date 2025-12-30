use std::{sync::Arc, time::Duration};

use futures_util::{SinkExt, StreamExt};
use neuro_sama::game::Api;
use tokio::sync::mpsc;
use tokio_tungstenite::tungstenite::Message;

use crate::controller::Controller;

struct NeuroDesktop(mpsc::UnboundedSender<Message>, Controller);

use schemars::JsonSchema;
use serde::Deserialize;

#[allow(unused)]
#[derive(Debug, Deserialize, JsonSchema)]
pub struct ExecuteHLCommandScript {
    script_contents: String,
}

#[allow(unused)]
#[derive(Debug, Deserialize, JsonSchema)]
struct Action2 {
    a: u32,
    b: bool,
}

#[derive(Debug, Deserialize, JsonSchema)]
struct Action3;

#[derive(Debug, neuro_sama::derive::Actions)]
pub enum Action {
    /// Action 1 description
    #[allow(unused)]
    #[name = "ExecuteHLCommandScript"]
    ExecuteHLCommandScript(ExecuteHLCommandScript),
    /// Action 2 description
    #[name = "action2"]
    Action2(Action2),
    /// Action 3 description
    #[name = "action3"]
    Action3(Action3),
}
impl neuro_sama::game::Game for NeuroDesktop {
    const NAME: &'static str = "Neuro Desktop";
    type Actions<'a> = Action;
    fn send_command(&self, message: Message) {
        let _ = self.0.send(message);
    }
    fn reregister_actions(&self) {
        // your game could have some complicated logic here i guess
        self.register_actions::<Action>().unwrap();
    }
    fn handle_action<'a>(
        &self,
        action: Self::Actions<'a>,
    ) -> Result<
        Option<impl 'static + Into<std::borrow::Cow<'static, str>>>,
        Option<impl 'static + Into<std::borrow::Cow<'static, str>>>,
    > {
        match action {
            Action::Action3(_) => Err(Some("try again")),
            // This action is for manual execution of a provided command script.
            // While normally, an higher level system will be used.

            // That system takes an description of what neuro / evil would want
            // to do, in ENGLISH. and compiles it, into an command script.
            Action::ExecuteHLCommandScript(act) => {
                // Execute High Level Command Script for neuro desktop
                self.1
                    .run_script(&act.script_contents)
                    .map_err(|_| Some("script failed"))?;

                Ok(Some("script executed".to_string()))
            },
            Action::Action2(act) => {
                if act.b {
                    Ok(Some("ok".to_string()))
                } else {
                    Err(Some("err"))
                }
            }
        }
    }
}

#[tokio::main(flavor = "current_thread")]
pub async fn start_integration(controller: Controller) {
    let (game2ws_tx, mut game2ws_rx) = mpsc::unbounded_channel();
    let game = Arc::new(NeuroDesktop(game2ws_tx, controller));
    game.initialize().unwrap();
    let game1 = game.clone();
    tokio::spawn(async move {
        loop {
            tokio::time::sleep(Duration::from_secs(20)).await;
            game1
                .force_actions::<Action>("do your thing".into())
                .with_state("some state idk")
                .send()
                .unwrap();
        }
    });
    let mut ws =
        tokio_tungstenite::connect_async(if let Ok(url) = std::env::var("NEURO_SDK_WS_URL") {
            url
        } else {
            "ws://127.0.0.1:8000".to_owned()
        })
        .await
        .unwrap()
        .0;
    loop {
        tokio::select! {
            msg = game2ws_rx.recv() => {
                println!("game2ws {msg:?}");
                let Some(msg) = msg else {
                    break;
                };
                if ws.send(msg).await.is_err() {
                    println!("websocket send failed");
                    break;
                }
            }
            msg = ws.next() => {
                println!("ws2game {msg:?}");
                let Some(msg) = msg else {
                    break;
                };
                let Ok(msg) = msg else {
                    continue;
                };
                if let Err(err) = game.handle_message(msg) {
                    // this could happen because we don't know what this message means (e.g. added
                    // in a new version of the API)
                    println!("notify_message failed: {err}");
                    continue;
                }
            }

        }
    }
}
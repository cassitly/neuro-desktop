use futures_util::{SinkExt, StreamExt};
use tokio::sync::mpsc;
use tokio_tungstenite::{connect_async, tungstenite::Message};
use url::Url;

use neuro_sama::game::{
    GameMessage,
    RegisterActions,
    ActionResult,
};

/// Message YOU send TO Neuro
#[derive(Debug)]
pub enum NeuroInput {
    Context(String),
    ActionResult {
        action: String,
        result: String,
    },
}

/// Message Neuro sends TO YOU
#[derive(Debug)]
pub struct NeuroAction {
    pub action: String,
    pub data: serde_json::Value,
}

/// Starts the Neuro integration in the background
///
/// Returns:
/// - Sender: send context / results to Neuro
/// - Receiver: receive actions from Neuro
pub async fn start_integration(
    game_name: &str,
    ws_url: &str,
) -> (
    mpsc::Sender<NeuroInput>,
    mpsc::Receiver<NeuroAction>,
) {
    let (to_neuro_tx, mut to_neuro_rx) = mpsc::channel::<NeuroInput>(32);
    let (from_neuro_tx, from_neuro_rx) = mpsc::channel::<NeuroAction>(32);

    let game_name = game_name.to_string();
    let ws_url = ws_url.to_string();

    tokio::spawn(async move {
        let url = Url::parse(&ws_url).expect("Invalid Neuro WS URL");
        let (ws, _) = connect_async(url).await.expect("Neuro connect failed");

        let (mut write, mut read) = ws.split();

        // Register actions once
        let register = RegisterActions {
            game: game_name.clone(),
            actions: vec![
                ("move".into(), "Move somewhere".into()),
                ("attack".into(), "Attack a target".into()),
                ("wait".into(), "Do nothing".into()),
            ],
        };

        write
            .send(Message::Text(
                serde_json::to_string(&GameMessage::RegisterActions(register)).unwrap(),
            ))
            .await
            .unwrap();

        loop {
            tokio::select! {
                // Incoming messages FROM Neuro
                msg = read.next() => {
                    let Some(Ok(Message::Text(text))) = msg else { continue };

                    if let Ok(parsed) = serde_json::from_str::<GameMessage>(&text) {
                        if let GameMessage::Action { action, data } = parsed {
                            let _ = from_neuro_tx.send(NeuroAction {
                                action,
                                data,
                            }).await;
                        }
                    }
                }

                // Messages FROM your game
                Some(input) = to_neuro_rx.recv() => {
                    let msg = match input {
                        NeuroInput::Context(text) => {
                            GameMessage::Context {
                                game: game_name.clone(),
                                context: text,
                            }
                        }
                        NeuroInput::ActionResult { action, result } => {
                            GameMessage::ActionResult(ActionResult {
                                game: game_name.clone(),
                                action,
                                result,
                            })
                        }
                    };

                    write
                        .send(Message::Text(serde_json::to_string(&msg).unwrap()))
                        .await
                        .unwrap();
                }
            }
        }
    });

    (to_neuro_tx, from_neuro_rx)
}

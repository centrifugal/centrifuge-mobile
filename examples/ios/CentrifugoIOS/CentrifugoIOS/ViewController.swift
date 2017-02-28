//
//  ViewController.swift
//  CentrifugoIOS
//
//  Created by Alexander Emelin on 25/02/2017.
//  Copyright © 2017 Alexander Emelin. All rights reserved.
//

import UIKit
import Centrifuge

class ConnectHandler : NSObject, CentrifugeConnectHandlerProtocol {
    var l: UILabel!
    
    func setLabel(l: UILabel!) {
        self.l = l
    }
    
    func onConnect(_ p0: CentrifugeClient!) {
        DispatchQueue.main.async{
            self.l.text = "Connected";
        }
    }
}

class DisconnectHandler : NSObject, CentrifugeDisconnectHandlerProtocol {
    var l: UILabel!
    
    func setLabel(l: UILabel!) {
        self.l = l
    }
    
    func onDisconnect(_ p0: CentrifugeClient!) {
        DispatchQueue.main.async{
            self.l.text = "Disconnected";
        }
    }
}

class MessageHandler : NSObject, CentrifugeMessageHandlerProtocol {
    var l: UILabel!
    
    func setLabel(l: UILabel!) {
        self.l = l
    }
    
    func onMessage(_ p0: CentrifugeSub!, p1: CentrifugeMessage!) throws {
        DispatchQueue.main.async{
            self.l.text = p1.data()
        }
    }
}

class ViewController: UIViewController {
    
    @IBOutlet weak var label: UILabel!
    
    override func viewDidLoad() {
        super.viewDidLoad()
        
        label.text = "Connecting..."
        
        DispatchQueue.main.async{
            let creds = CentrifugeNewCredentials(
                "42", "1488055494", "", "24d0aa4d7c679e45e151d268044723d07211c6a9465d0e35ee35303d13c5eeff"
            )
            
            let eventHandler = CentrifugeNewEventHandler()
            let connectHandler = ConnectHandler()
            connectHandler.setLabel(l: self.label)
            let disconnectHandler = DisconnectHandler()
            disconnectHandler.setLabel(l: self.label)
            
            eventHandler?.onConnect(connectHandler)
            eventHandler?.onDisconnect(disconnectHandler)
            
            
            let url = "ws://localhost:8000/connection/websocket"
            let client = CentrifugeNew(url, creds, eventHandler, CentrifugeDefaultConfig())
            
            do {
                try client?.connect()
            } catch {
                self.label.text = "Error on connect..."
                return
            }
            
            self.label.text = "Connected"
            
            let subEventHandler = CentrifugeNewSubEventHandler()
            let messageHandler = MessageHandler()
            messageHandler.setLabel(l: self.label)
            subEventHandler?.onMessage(messageHandler)
            
            let sub: CentrifugeSub?
            do {
                sub = try client?.subscribe("public:chat", events: subEventHandler)
            } catch {
                self.label.text = "Error on subscribe"
                return
            }
            
            DispatchQueue.main.asyncAfter(deadline: .now() + .seconds(3), execute: {
                do {
                    let data = "{\"input\": \"hello\"}".data(using: .utf8)
                    try sub?.publish(data)
                } catch {
                    self.label.text = "Publish error"
                    return
                }
            })
        }
    }
    
    override func didReceiveMemoryWarning() {
        super.didReceiveMemoryWarning()
        // Dispose of any resources that can be recreated.
    }
    
    
}

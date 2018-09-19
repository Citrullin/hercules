package utils

import (
	"gitlab.com/semkodev/hercules/config"
	"gitlab.com/semkodev/hercules/logs"
)

func Hello() {
	if !config.AppConfig.GetBool("log.hello") {
		return
	}

	logs.Log.Info(`


                                                                                    *..((/@%,%/#
                                                                                       .  .&%.%**    .
                                                                                           #%.  .@@(,
                                                                                              .*@@@@@@%
                                                                                         **  .@ @@@@&@%
                                                                                         .   @@*&@@@%
                                                                                             @(@@@@ .@
                                                                                            .(  ,
                                                                                  ,        **(@    *@
                                                                                     * %&  .      @@@.
                                                          %@@@@@@#,#% *                          .,(%
                                                       @@*@@@@@%@@@@@ .                  .   %@@#.
                                                     *@&@@@@@@@ @@@@%,.        ,.
                                              ,&@@@#@/*,*%@@*@,@@%.                               %*/
                                            ,    ##@@@@@@@*#@%@                                    ##
                                               ,#@@@@@@@@@@@.*@  *@@%
                                             .**@@@@@@@@@@@@@ %@%@@@@@@@@.,                    .    ./@,
                                             *  &@@@@@@@@@@@@,*@%@@@@@@@@@@@#                   @&@.*(@@@@
                                          . (   ,/@@@@@@@@@@@, @&@@@@@@@@@@@@@      *&*#        @@@@@%@@@@@
                                                   ,(@(#%@@@@* %@*@@@@@@@@@@@@%         (@@@%@@@@@@@@@@@@@@@
                                                          .@@%*,@,#@@@@@@@@@@@&          #%@&@@@@.%@@@@@@@@@#
                                          (                 %   ./.%@(&@&@%.          .*&&@@@@(      &@@@@@
                                         .#                       ,                               *      .,%@
                                          ,                                    ..(@@@%.
                                         ,%                                 ..    #*&@@@@#.
                     ,                    ...    .                                                     ,%@  *
                                             (@.                              .,  *,(                  %@@@@@.
                                           /@@  .                                                      /&%@@@/
                      ,                        /                                                       #@%@@@,
                                            .                                                          @@@@@@@%
                                         .&                                                             #@@@@@%
                                         * *                                                  .          %@@@@%
                                       * .@.                                                              &@@@@
                                       ,@@@@%.                                                             #&/@,
                                  /    @@@@@@@#                                                ..           ,@@@.
                          .           .@@@@##@@,                                             @.            (@@@@@
              ,                        @@@@@ @@.                                                          @@@@@@@
                                      ,&&@@@@*@                                                 .    .%@@@@@@@@@@.
                                     *(&@@@@@*,                                     .           .        @@@@@@@@,
                                       &@&@@@%                                                 *#        %@@@@@/@.      .
                                      &@@@@@@                                               .         /((,@@@@@%%
                     .               (& (@*                                                               @@@@@@@%.
	`)
	logs.Log.Info("db   db d88888b d8888b.  .o88b. db    db db      d88888b .d8888.")
	logs.Log.Info("88   88 88'     88  `8D d8P  Y8 88    88 88      88'     88'  YP")
	logs.Log.Info("88ooo88 88ooooo 88oobY' 8P      88    88 88      88ooooo `8bo.")
	logs.Log.Info("88~~~88 88~~~~~ 88`8b   8b      88    88 88      88~~~~~   `Y8b.")
	logs.Log.Info("88   88 88.     88 `88. Y8b  d8 88b  d88 88booo. 88.     db   8D")
	logs.Log.Info("YP   YP Y88888P 88   YD  `Y88P' ~Y8888P' Y88888P Y88888P `8888Y'")
}
